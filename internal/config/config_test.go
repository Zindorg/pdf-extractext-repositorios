package config_test

import (
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadWithDefaults(t *testing.T) {
	t.Setenv("HTTP_PORT", "")
	t.Setenv("MONGODB_URI", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("STREAM_NAME", "")
	t.Setenv("STREAM_GROUP", "")
	t.Setenv("DLQ_NAME", "")
	t.Setenv("RETRY_MAX", "")
	t.Setenv("RETRY_BACKOFF", "")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, "8083", cfg.HTTPPort)
	require.Equal(t, "mongodb://localhost:27017/text_extractor_db", cfg.MongoDBURI)
	require.Equal(t, "localhost:6379", cfg.RedisAddr)
	require.Equal(t, "document-events", cfg.StreamName)
	require.Equal(t, "documents-persister", cfg.StreamGroup)
	require.Equal(t, "document-events-dlq", cfg.DLQName)
	require.Equal(t, 5, cfg.RetryMax)
	require.Equal(t, 5*time.Second, cfg.RetryBackoff)
	require.Equal(t, int64(50<<20), cfg.MaxTextBytes)
	require.Equal(t, "info", cfg.LogLevel)
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("HTTP_PORT", "9999")
	t.Setenv("MONGODB_URI", "mongodb://mongo:27017/custom")
	t.Setenv("MONGODB_DATABASE", "custom")
	t.Setenv("REDIS_ADDR", "redis:6380")
	t.Setenv("STREAM_NAME", "custom-events")
	t.Setenv("STREAM_GROUP", "custom-group")
	t.Setenv("DLQ_NAME", "custom-dlq")
	t.Setenv("RETRY_MAX", "10")
	t.Setenv("RETRY_BACKOFF", "2s")
	t.Setenv("MAX_TEXT_BYTES", "1048576")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, "9999", cfg.HTTPPort)
	require.Equal(t, "mongodb://mongo:27017/custom", cfg.MongoDBURI)
	require.Equal(t, "custom", cfg.MongoDBDatabase)
	require.Equal(t, "redis:6380", cfg.RedisAddr)
	require.Equal(t, "custom-events", cfg.StreamName)
	require.Equal(t, "custom-group", cfg.StreamGroup)
	require.Equal(t, "custom-dlq", cfg.DLQName)
	require.Equal(t, 10, cfg.RetryMax)
	require.Equal(t, 2*time.Second, cfg.RetryBackoff)
	require.Equal(t, int64(1048576), cfg.MaxTextBytes)
	require.Equal(t, "debug", cfg.LogLevel)
}

func TestLoadInvalidRetryMax(t *testing.T) {
	t.Setenv("RETRY_MAX", "abc")

	_, err := config.Load()
	require.Error(t, err)
}