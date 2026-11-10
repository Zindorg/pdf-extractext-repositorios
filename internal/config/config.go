package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/logging"
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
	defaultMaxTextBytes    = 15 << 20
	defaultLogLevel        = "info"
	defaultRateLimitRPS    = 100
	defaultRateLimitBurst  = 200
	defaultCBFailures      = 5
	defaultCBTimeout       = 30 * time.Second
)

type Config struct {
	HTTPPort        string
	MongoDBURI      string
	MongoDBDatabase string
	RedisAddr       string
	StreamName      string
	StreamGroup     string
	DLQName         string
	ConsumerName    string
	RetryMax        int
	RetryBackoff    time.Duration
	MaxTextBytes    int64
	LogLevel        string
	RateLimitRPS    int
	RateLimitBurst  int
	CBFailures      int
	CBTimeout       time.Duration
}

// LogLevelParsed returns the parsed slog.Level from LogLevel.
func (c *Config) LogLevelParsed() slog.Level {
	return logging.ParseLevel(c.LogLevel)
}

func Load() (*Config, error) {
	cfg := stringConfig()
	if err := loadNumbers(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// defaultConsumerName genera un identificador único por instancia para
// XREADGROUP/XAUTOCLAIM: hostname-pid permite réplicas horizontales.
func defaultConsumerName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "persister"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
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
		ConsumerName:    env("CONSUMER_NAME", defaultConsumerName()),
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
	if cfg.RateLimitRPS, err = envParse("RATE_LIMIT_RPS", defaultRateLimitRPS, "integer", strconv.Atoi); err != nil {
		return fmt.Errorf("RATE_LIMIT_RPS: %w", err)
	}
	if cfg.RateLimitBurst, err = envParse("RATE_LIMIT_BURST", defaultRateLimitBurst, "integer", strconv.Atoi); err != nil {
		return fmt.Errorf("RATE_LIMIT_BURST: %w", err)
	}
	if cfg.CBFailures, err = envParse("CB_FAILURES", defaultCBFailures, "integer", strconv.Atoi); err != nil {
		return fmt.Errorf("CB_FAILURES: %w", err)
	}
	if cfg.CBTimeout, err = envParse("CB_TIMEOUT", defaultCBTimeout, "duration", time.ParseDuration); err != nil {
		return fmt.Errorf("CB_TIMEOUT: %w", err)
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
