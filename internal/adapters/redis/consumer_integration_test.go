//go:build integration

package redis

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// newIntegrationConsumer arranca un consumer sobre Redis real, con streams
// únicos por test para no colisionar entre ejecuciones.
func newIntegrationConsumer(t *testing.T, retryMax int, backoff time.Duration) (*StreamConsumer, *redis.Client, *testutil.MemoryRepository) {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		t.Skip("Redis no disponible para tests de integración")
		return nil, nil, nil
	}
	t.Cleanup(func() { _ = client.Close() })

	repo := testutil.NewMemoryRepository()
	service := application.NewDocumentService(repo)
	suffix := time.Now().UnixNano()
	stream := fmt.Sprintf("it-events-%d", suffix)
	group := fmt.Sprintf("it-group-%d", suffix)
	dlq := fmt.Sprintf("it-dlq-%d", suffix)
	consumer, err := NewStreamConsumer(client, stream, group, dlq, service, NewRetryDLQ(retryMax, backoff))
	require.NoError(t, err)
	require.NoError(t, consumer.ensureGroup(context.Background()))
	return consumer, client, repo
}

// publish añade un evento JSON al stream del consumer.
func publish(t *testing.T, client *redis.Client, consumer *StreamConsumer, payload string) {
	t.Helper()
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: consumer.streamName,
		Values: map[string]any{streamPayloadField: payload},
	}).Result()
	require.NoError(t, err)
}

// pendingLen cuenta los mensajes pendientes (sin ACK) del grupo.
func pendingLen(t *testing.T, client *redis.Client, consumer *StreamConsumer) int64 {
	t.Helper()
	pending, err := client.XPending(context.Background(), consumer.streamName, consumer.streamGroup).Result()
	require.NoError(t, err)
	return pending.Count
}

const integOriginal = `{
	"event_type": "original",
	"schema_version": 1,
	"document_id": "doc-1",
	"checksum": "chk-1",
	"extracted_text": "texto",
	"extraction_time_ms": 342,
	"mime_type": "application/pdf",
	"metadata": {"filename": "x.pdf"}
}`

func TestConsumer_Integration_Original_CreatesPendingAndAcks(t *testing.T) {
	consumer, client, repo := newIntegrationConsumer(t, 5, 50*time.Millisecond)

	publish(t, client, consumer, integOriginal)
	require.NoError(t, consumer.consume(context.Background()))

	doc, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusPending, doc.Status)
	require.Equal(t, int64(342), doc.ExtractionTimeMS)
	require.Equal(t, "application/pdf", doc.Metadata.MimeType)
	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_Original_Duplicate_AcksAndDiscards(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 5, 50*time.Millisecond)

	publish(t, client, consumer, integOriginal)
	require.NoError(t, consumer.consume(context.Background()))
	publish(t, client, consumer, integOriginal)
	require.NoError(t, consumer.consume(context.Background()))

	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_SummaryResolved_Completes(t *testing.T) {
	consumer, client, repo := newIntegrationConsumer(t, 5, 50*time.Millisecond)

	publish(t, client, consumer, integOriginal)
	require.NoError(t, consumer.consume(context.Background()))

	publish(t, client, consumer, `{
		"event_type": "summary_resolved",
		"schema_version": 1,
		"document_id": "doc-1",
		"summary": "resumen",
		"summary_time_ms": 120
	}`)
	require.NoError(t, consumer.consume(context.Background()))

	doc, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, doc.Status)
	require.Equal(t, "resumen", *doc.Summary)
	require.Equal(t, int64(120), doc.SummaryTimeMS)
	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_OrphanSummary_GoesToDLQ(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 1, 50*time.Millisecond)

	publish(t, client, consumer, `{
		"event_type": "summary_resolved",
		"schema_version": 1,
		"document_id": "no-existe",
		"summary": "huérfano",
		"summary_time_ms": 5
	}`)

	require.NoError(t, consumer.consume(context.Background()))

	require.Zero(t, pendingLen(t, client, consumer))
	entries, err := client.XRange(context.Background(), consumer.dlqName, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Values["error"], "document not found")
}

func TestConsumer_Integration_InvalidEvent_RetriesThenDLQ(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 2, 100*time.Millisecond)

	publish(t, client, consumer, `{
		"event_type": "original",
		"schema_version": 99,
		"document_id": "doc-1",
		"checksum": "chk-1",
		"extracted_text": "texto",
		"mime_type": "application/pdf",
		"metadata": {"filename": "x.pdf"}
	}`)

	require.NoError(t, consumer.consume(context.Background()))
	require.Equal(t, int64(1), pendingLen(t, client, consumer))

	time.Sleep(200 * time.Millisecond)
	require.NoError(t, consumer.consume(context.Background()))

	require.Zero(t, pendingLen(t, client, consumer))
	entries, err := client.XRange(context.Background(), consumer.dlqName, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "2", entries[0].Values["attempts"])
}

func TestConsumer_Integration_InvalidJSON_GoesToDLQ(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 1, 50*time.Millisecond)

	publish(t, client, consumer, "{no válido")

	require.NoError(t, consumer.consume(context.Background()))

	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_Run_StopsOnCancel(t *testing.T) {
	consumer, _, _ := newIntegrationConsumer(t, 5, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumer.Run(ctx) }()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Run no retornó tras cancelar ctx")
	}
}
