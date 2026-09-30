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
	retryMax, err := envInt("RETRY_MAX", defaultRetryMax)
	if err != nil {
		return nil, fmt.Errorf("RETRY_MAX: %w", err)
	}

	retryBackoff, err := envDuration("RETRY_BACKOFF", defaultRetryBackoff)
	if err != nil {
		return nil, fmt.Errorf("RETRY_BACKOFF: %w", err)
	}

	maxTextBytes, err := envInt64("MAX_TEXT_BYTES", defaultMaxTextBytes)
	if err != nil {
		return nil, fmt.Errorf("MAX_TEXT_BYTES: %w", err)
	}

	return &Config{
		HTTPPort:        env("HTTP_PORT", defaultHTTPPort),
		MongoDBURI:      env("MONGODB_URI", defaultMongoDBURI),
		MongoDBDatabase: env("MONGODB_DATABASE", defaultMongoDBDatabase),
		RedisAddr:       env("REDIS_ADDR", defaultRedisAddr),
		StreamName:      env("STREAM_NAME", defaultStreamName),
		StreamGroup:     env("STREAM_GROUP", defaultStreamGroup),
		DLQName:         env("DLQ_NAME", defaultDLQName),
		RetryMax:        retryMax,
		RetryBackoff:    retryBackoff,
		MaxTextBytes:    maxTextBytes,
		LogLevel:        env("LOG_LEVEL", defaultLogLevel),
	}, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", v, err)
	}
	return n, nil
}

func envInt64(key string, fallback int64) (int64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", v, err)
	}
	return n, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", v, err)
	}
	return d, nil
}
