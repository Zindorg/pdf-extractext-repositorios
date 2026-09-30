package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

// depsHealth agrupa los ping de dependencias para /health.
type depsHealth struct {
	mongo *mongo.Client
	redis *redis.Client
}

func (h *depsHealth) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := h.mongo.Ping(ctx, readpref.Primary()); err != nil {
		return err
	}
	return h.redis.Ping(ctx).Err()
}

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

	// --- Mongo ---------------------------------------------------------
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoDBURI))
	if err != nil {
		return err
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()

	// verificación temprana: si Mongo no responde, no arrancamos
	{
		ctx, cancel := context.WithTimeout(rootCtx, 10*time.Second)
		defer cancel()
		if err := mongoClient.Ping(ctx, readpref.Primary()); err != nil {
			return err
		}
	}

	db := mongoClient.Database(cfg.MongoDBDatabase)
	repo := mongodb.NewMongoDocumentRepository(db)

	ctx, cancel := context.WithTimeout(rootCtx, 10*time.Second)
	defer cancel()
	if err := repo.EnsureIndexes(ctx); err != nil {
		return err
	}

	// --- Redis ---------------------------------------------------------
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() { _ = redisClient.Close() }()

	{
		ctx, cancel := context.WithTimeout(rootCtx, 10*time.Second)
		defer cancel()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return err
		}
	}

	// --- Wiring de la API ----------------------------------------------
	service := application.NewDocumentService(repo)

	consumer, err := redisadapter.NewStreamConsumer(rootCtx, redisClient, cfg.StreamName, cfg.StreamGroup, service)
	if err != nil {
		return err
	}
	_ = consumer // la fase de lógica conecta consumer ↔ service

	handler := api.NewDocumentHandler(service, &depsHealth{mongo: mongoClient, redis: redisClient})
	router := api.NewRouter(handler)

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// --- Serve + graceful shutdown --------------------------------------
	errCh := make(chan error, 1)
	go func() {
		log.Printf("servidor HTTP escuchando en :%s", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-rootCtx.Done():
		log.Println("apagando…")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	return server.Shutdown(shutdownCtx)
}
