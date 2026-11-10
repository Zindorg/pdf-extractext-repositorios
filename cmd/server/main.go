package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/adapters/health"
	"github.com/marco/pdf-extractext-repositorios/internal/adapters/mongodb"
	redisadapter "github.com/marco/pdf-extractext-repositorios/internal/adapters/redis"
	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/config"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/logging"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"log/slog"
)

const (
	startupTimeout    = 10 * time.Second // ping e índices al arrancar
	readHeaderTimeout = 5 * time.Second  // hardening: límite de lectura de headers
	shutdownTimeout   = 10 * time.Second // graceful shutdown
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	// Setup structured logging
	logging.Setup(cfg.LogLevelParsed())
	slog.Info("starting persister", "version", "dev")

	if err := run(cfg); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config) error {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	inf, err := startInfra(rootCtx, cfg)
	if err != nil {
		return err
	}
	defer inf.close()

	return serveHTTP(rootCtx, cfg, inf)
}

// infra agrupa los clientes de infraestructura; close() libera ambos.
type infra struct {
	mongo *mongo.Client
	redis *redis.Client
	repo  domain.DocumentRepository
}

func (i *infra) close() {
	_ = i.mongo.Disconnect(context.Background())
	_ = i.redis.Close()
}

// startInfra conecta Mongo (ping + índices) y Redis (ping).
func startInfra(ctx context.Context, cfg *config.Config) (*infra, error) {
	mongoClient, repo, err := startMongo(ctx, cfg.MongoDBURI, cfg.MongoDBDatabase, cfg)
	if err != nil {
		return nil, err
	}
	redisClient, err := startRedis(ctx, cfg.RedisAddr)
	if err != nil {
		_ = mongoClient.Disconnect(context.Background())
		return nil, err
	}
	return &infra{mongo: mongoClient, redis: redisClient, repo: repo}, nil
}

// startMongo conecta, verifica tempranamente (si Mongo no responde, no
// arrancamos) y prepara los índices. El dueño del cliente es el caller.
func startMongo(ctx context.Context, uri, dbName string, cfg *config.Config) (*mongo.Client, domain.DocumentRepository, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, fmt.Errorf("mongo connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("mongo ping: %w", err)
	}
	baseRepo := mongodb.NewMongoDocumentRepository(client.Database(dbName))
	idxCtx, idxCancel := context.WithTimeout(ctx, startupTimeout)
	defer idxCancel()
	if err := baseRepo.EnsureIndexes(idxCtx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("mongo indexes: %w", err)
	}

	cb := mongodb.NewCircuitBreaker(cfg.CBFailures, 2, cfg.CBTimeout)
	repo := mongodb.NewCircuitBreakerRepository(baseRepo, cb)

	return client, repo, nil
}

// startRedis conecta el cliente y verifica que el servidor responda.
func startRedis(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	pingCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return client, nil
}

// serveHTTP monta la API, arranca el servidor y bloquea hasta la señal de
// apagado o un error fatal del listener.
func serveHTTP(rootCtx context.Context, cfg *config.Config, inf *infra) error {
	service := application.NewDocumentService(inf.repo)
	if err := startConsumer(rootCtx, cfg, inf, service); err != nil {
		return err
	}

	server := newServer(cfg, service, inf)
	slog.Info("servidor HTTP escuchando", "port", cfg.HTTPPort)
	errCh := listenAndServe(server)

	select {
	case err := <-errCh:
		return err
	case <-rootCtx.Done():
		slog.Info("apagando…")
	}
	return shutdown(server)
}

// startConsumer arranca el consumer del stream en background. Un fallo al
// crear el consumer group es fatal en el arranque; los fallos del loop los
// gestiona el propio consumer (retry → DLQ).
func startConsumer(ctx context.Context, cfg *config.Config, inf *infra, service *application.DocumentService) error {
	retry := redisadapter.NewRetryDLQ(cfg.RetryMax, cfg.RetryBackoff)
	consumerCfg := redisadapter.Config{
		StreamName:   cfg.StreamName,
		StreamGroup:  cfg.StreamGroup,
		DLQName:      cfg.DLQName,
		ConsumerName: cfg.ConsumerName,
		MaxTextBytes: cfg.MaxTextBytes,
	}
	consumer, err := redisadapter.NewStreamConsumer(inf.redis, consumerCfg, service, retry)
	if err != nil {
		return fmt.Errorf("redis consumer: %w", err)
	}
	go func() {
		if err := consumer.Run(ctx); err != nil {
			slog.Error("stream consumer stopped", "error", err)
		}
	}()
	return nil
}

// newServer construye el servidor HTTP: service → handler → router.
func newServer(cfg *config.Config, service *application.DocumentService, inf *infra) *http.Server {
	handler := api.NewDocumentHandler(service, health.NewPinger(inf.mongo, inf.redis))
	return &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           api.NewRouter(handler, cfg.RateLimitRPS, cfg.RateLimitBurst),
		ReadHeaderTimeout: readHeaderTimeout,
	}
}

// listenAndServe sirve en background y propaga errores fatales del listener.
func listenAndServe(server *http.Server) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	return errCh
}

// shutdown apaga el servidor con timeout (context.Background: rootCtx ya
// está cancelado en este punto).
func shutdown(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		return err
	}
	slog.Info("server stopped gracefully")
	return nil
}
