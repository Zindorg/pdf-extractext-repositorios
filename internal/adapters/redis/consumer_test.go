package redis

import (
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestNewStreamConsumer_UsaNombreConfigurado(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	repo := testutil.NewMemoryRepository()
	service := application.NewDocumentService(repo)
	retry := NewRetryDLQ(5, 50*1e6)

	cfg := Config{
		StreamName:   "test-stream",
		StreamGroup:  "test-group",
		DLQName:      "test-dlq",
		ConsumerName: "c-replica-1",
		MaxTextBytes: 1 << 20,
	}
	consumer, err := NewStreamConsumer(client, cfg, service, retry)
	require.NoError(t, err)
	require.Equal(t, "c-replica-1", consumer.consumerName)
	require.Equal(t, "test-stream", consumer.streamName)
	require.Equal(t, "test-group", consumer.streamGroup)
	require.Equal(t, "test-dlq", consumer.dlqName)
	require.Equal(t, int64(1<<20), consumer.maxTextBytes)
	require.NoError(t, client.Close())
}

func TestNewStreamConsumer_NilClient(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	service := application.NewDocumentService(repo)
	retry := NewRetryDLQ(5, 50*1e6)

	cfg := Config{
		StreamName:   "test-stream",
		StreamGroup:  "test-group",
		DLQName:      "test-dlq",
		ConsumerName: "c-replica-1",
		MaxTextBytes: 1 << 20,
	}
	_, err := NewStreamConsumer(nil, cfg, service, retry)
	require.ErrorIs(t, err, errNotConnected)
}
