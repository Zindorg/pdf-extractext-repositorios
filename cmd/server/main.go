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

	"github.com/marco/pdf-extractext-repositorios/internal/adapters/cache"
	"github.com/marco/pdf-extractext-repositorios/internal/adapters/health"
	"github.com/marco/pdf-extractext-repositorios/internal/adapters/mongodb"
	redisadapter "github.com/marco/pdf-extractext-repositorios/internal/adapters/redis"
	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/config"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/logging"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"log/slog"
)

const (
	startupTimeout    = 10 * time.Second  // ping e índices al arrancar
	readHeaderTimeout = 5 * time.Second   // hardening: límite de lectura de headers
	readTimeout       = 10 * time.Second  // hardening: límite de lectura del body
	writeTimeout      = 30 * time.Second  // hardening: límite de escritura de respuesta
	idleTimeout       = 120 * time.Second // hardening: límite de conexiones idle
	shutdownTimeout   = 10 * time.Second  // graceful shutdown
)

// CLI subcommands para admin processes (12 Factor XII)
func runMigrate(cfg *config.Config) error {
	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		return fmt.Errorf("mongo connect: %w", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(cfg.MongoDBDatabase)
	repo := mongodb.NewMongoDocumentRepository(db)
	return repo.EnsureIndexes(ctx)
}

func runReindex(cfg *config.Config) error {
	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		return fmt.Errorf("mongo connect: %w", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(cfg.MongoDBDatabase)
	// Drop indexes
	indexes := []string{"uq_document_id", "uq_checksum_active", "ix_filename", "ix_created_at", "ix_summary_status"}
	for _, name := range indexes {
		_ = db.Collection("documents").Indexes().DropOne(ctx, name)
	}
	// Recreate
	repo := mongodb.NewMongoDocumentRepository(db)
	return repo.EnsureIndexes(ctx)
}

func runCleanupDLQ(cfg *config.Config, requeue bool) error {
	ctx := context.Background()
	opts := &redis.Options{
		Addr:     cfg.RedisAddr,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
	}
	client := redis.NewClient(opts)
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	dlq := cfg.DLQName
	if dlq == "" {
		dlq = "document-events-dlq"
	}

	// Listar mensajes
	msgs, err := client.XRange(ctx, dlq, "-", "+").Result()
	if err != nil {
		return fmt.Errorf("xrange dlq: %w", err)
	}

	fmt.Printf("DLQ %s: %d mensajes\n", dlq, len(msgs))
	for _, m := range msgs {
		fmt.Printf("  ID: %s, document_id: %s\n", m.ID, m.Values["document_id"])
	}

	if requeue && len(msgs) > 0 {
		stream := cfg.StreamName
		if stream == "" {
			stream = "document-events"
		}
		for _, m := range msgs {
			payload := m.Values["payload"]
			docID := m.Values["document_id"]
			err := client.XAdd(ctx, &redis.XAddArgs{
				Stream: stream,
				Values: map[string]any{"document_id": docID, "payload": payload},
			}).Err()
			if err != nil {
				return fmt.Errorf("requeue %s: %w", docID, err)
			}
			_ = client.XDel(ctx, dlq, m.ID).Err()
		}
		fmt.Printf("Re-encolados %d mensajes a %s\n", len(msgs), stream)
	}
	return nil
}

func runRestoreByChecksum(cfg *config.Config, checksum string) error {
	if checksum == "" {
		return fmt.Errorf("checksum requerido")
	}
	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		return fmt.Errorf("mongo connect: %w", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(cfg.MongoDBDatabase)

	// Buscar doc soft-deleted con ese checksum
	filter := bson.M{"checksum": checksum, "deleted_at": bson.M{"$ne": nil}}
	var pd struct {
		DocumentID string `bson:"document_id"`
	}
	err = db.Collection("documents").FindOne(ctx, filter).Decode(&pd)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("no hay documento soft-deleted con checksum %s", checksum)
		}
		return fmt.Errorf("find: %w", err)
	}

	// Restore via repo
	domainRepo := mongodb.NewMongoDocumentRepository(db)
	return domainRepo.Restore(ctx, pd.DocumentID)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	// Setup structured logging
	logging.Setup(cfg.LogLevelParsed())
	slog.Info("starting persister", "version", "dev")

	// Subcomandos admin (12 Factor XII)
	if len(os.Args) > 1 {
		var err error
		switch os.Args[1] {
		case "migrate":
			err = runMigrate(cfg)
		case "reindex":
			err = runReindex(cfg)
		case "cleanup-dlq":
			requeue := false
			if len(os.Args) > 2 && os.Args[2] == "--requeue" {
				requeue = true
			}
			err = runCleanupDLQ(cfg, requeue)
		case "restore-by-checksum":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: restore-by-checksum <checksum>")
				os.Exit(1)
			}
			err = runRestoreByChecksum(cfg, os.Args[2])
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
			os.Exit(1)
		}
		if err != nil {
			slog.Error("command failed", "error", err)
			os.Exit(1)
		}
		return
	}

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
	redisClient, err := startRedis(ctx, cfg)
	if err != nil {
		return nil, err
	}
	mongoClient, repo, err := startMongo(ctx, cfg.MongoDBURI, cfg.MongoDBDatabase, cfg, redisClient)
	if err != nil {
		_ = redisClient.Close()
		return nil, err
	}
	return &infra{mongo: mongoClient, redis: redisClient, repo: repo}, nil
}

// startMongo conecta, verifica tempranamente (si Mongo no responde, no
// arrancamos) y prepara los índices. El dueño del cliente es el caller.
func startMongo(ctx context.Context, uri, dbName string, cfg *config.Config, redisClient *redis.Client) (*mongo.Client, domain.DocumentRepository, error) {
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
	var repo domain.DocumentRepository = mongodb.NewCircuitBreakerRepository(baseRepo, cb)

	// Cache read-through (cache outermost, CB inner, Mongo innermost)
	repo = cache.NewCachedRepository(repo, redisClient, cfg.CacheTTL)

	return client, repo, nil
}

// startRedis conecta el cliente y verifica que el servidor responda.
func startRedis(ctx context.Context, cfg *config.Config) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:     cfg.RedisAddr,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
	}
	client := redis.NewClient(opts)
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
	if reg := consul.FromEnv(cfg.HTTPPort); reg.Enabled() {
	go reg.Register(rootCtx)
	defer reg.Deregister()
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
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
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
// runHealthcheck: para el HEALTHCHECK de Docker (la imagen distroless no tiene wget/curl).
func runHealthcheck() int {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8083"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/api/v1/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}