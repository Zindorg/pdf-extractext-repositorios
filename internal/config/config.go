package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultHTTPPort        = "8083"
	defaultMongoDBURI      = "mongodb://localhost:27017/text_extractor_db"
	defaultMongoDBDatabase = "text_extractor_db"
	defaultRedisAddr       = "localhost:6379"
	defaultStreamName      = "document-events"
	defaultStreamGroup     = "documents-persister"
	defaultDLQName         = "document-events-dlq"
	defaultRetryMax        = 5
	defaultRetryBackoff    = 5 * time.Second
	defaultMaxTextBytes    = 50 << 20
	defaultLogLevel        = "info"
)

type Config struct {
	HTTPPort        string
	MongoDBURI      string
	MongoDBDatabase string
	RedisAddr       string
	StreamName      string
	StreamGroup     string
	DLQName         string
	RetryMax        int
	RetryBackoff    time.Duration
	MaxTextBytes    int64
	LogLevel        string
}

func Load() (*Config, error) {
	cfg := stringConfig()
	if err := loadNumbers(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// stringConfig monta el bloque de variables de texto con sus defaults.
func stringConfig() *Config {
	return &Config{
		HTTPPort:        env("HTTP_PORT", defaultHTTPPort),
		MongoDBURI:      env("MONGODB_URI", defaultMongoDBURI),
		MongoDBDatabase: env("MONGODB_DATABASE", defaultMongoDBDatabase),
		RedisAddr:       env("REDIS_ADDR", defaultRedisAddr),
		StreamName:      env("STREAM_NAME", defaultStreamName),
		StreamGroup:     env("STREAM_GROUP", defaultStreamGroup),
		DLQName:         env("DLQ_NAME", defaultDLQName),
		LogLevel:        env("LOG_LEVEL", defaultLogLevel),
	}
}

// loadNumbers parsea las env vars numéricas y de duración.
func loadNumbers(cfg *Config) error {
	var err error
	if cfg.RetryMax, err = envParse("RETRY_MAX", defaultRetryMax, "integer", strconv.Atoi); err != nil {
		return fmt.Errorf("RETRY_MAX: %w", err)
	}
	if cfg.RetryBackoff, err = envParse("RETRY_BACKOFF", defaultRetryBackoff, "duration", time.ParseDuration); err != nil {
		return fmt.Errorf("RETRY_BACKOFF: %w", err)
	}
	if cfg.MaxTextBytes, err = envParse("MAX_TEXT_BYTES", defaultMaxTextBytes, "integer",
		func(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }); err != nil {
		return fmt.Errorf("MAX_TEXT_BYTES: %w", err)
	}
	return nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// envParse lee una env var y la convierte con parse; ausente o vacía → fallback.
// kind etiqueta el tipo para el mensaje de error ("integer", "duration", ...).
func envParse[T any](key string, fallback T, kind string, parse func(string) (T, error)) (T, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	out, err := parse(v)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("invalid %s %q: %w", kind, v, err)
	}
	return out, nil
}
