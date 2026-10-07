package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

const (
	startupTimeout    = 10 * time.Second // ping e índices al arrancar
	readHeaderTimeout = 5 * time.Second  // hardening: límite de lectura de headers
	shutdownTimeout   = 10 * time.Second // graceful shutdown
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

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
	repo  *mongodb.MongoDocumentRepository
}

func (i *infra) close() {
	_ = i.mongo.Disconnect(context.Background())
	_ = i.redis.Close()
}

// startInfra conecta Mongo (ping + índices) y Redis (ping).
func startInfra(ctx context.Context, cfg *config.Config) (*infra, error) {
	mongoClient, repo, err := startMongo(ctx, cfg.MongoDBURI, cfg.MongoDBDatabase)
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
func startMongo(ctx context.Context, uri, dbName string) (*mongo.Client, *mongodb.MongoDocumentRepository, error) {
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
	repo := mongodb.NewMongoDocumentRepository(client.Database(dbName))
	idxCtx, idxCancel := context.WithTimeout(ctx, startupTimeout)
	defer idxCancel()
	if err := repo.EnsureIndexes(idxCtx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("mongo indexes: %w", err)
	}
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
	log.Printf("servidor HTTP escuchando en :%s", cfg.HTTPPort)
	errCh := listenAndServe(server)

	select {
	case err := <-errCh:
		return err
	case <-rootCtx.Done():
		log.Println("apagando…")
	}
	return shutdown(server)
}

// startConsumer registra el consumer de eventos del stream.
func startConsumer(ctx context.Context, cfg *config.Config, inf *infra, service *application.DocumentService) error {
	consumer, err := redisadapter.NewStreamConsumer(ctx, inf.redis, cfg.StreamName, cfg.StreamGroup, service)
	if err != nil {
		return fmt.Errorf("redis consumer: %w", err)
	}
	_ = consumer // la fase de lógica conecta consumer ↔ service
	return nil
}

// newServer construye el servidor HTTP: service → handler → router.
func newServer(cfg *config.Config, service *application.DocumentService, inf *infra) *http.Server {
	handler := api.NewDocumentHandler(service, health.NewPinger(inf.mongo, inf.redis))
	return &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           api.NewRouter(handler),
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
	return server.Shutdown(ctx)
}
