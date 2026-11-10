//go:build integration

package redis

import (
	"context"
	"encoding/json"
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

// integMaxText es un límite holgado para los tests que no ejercitan
// MAX_TEXT_BYTES; el test del tope usa uno pequeño a propósito.
const integMaxText = 1 << 20

// newIntegrationConsumer arranca un consumer sobre Redis real, con streams
// únicos por test para no colisionar entre ejecuciones.
func newIntegrationConsumer(t *testing.T, retryMax int, backoff time.Duration, maxTextBytes int64, consumerName string) (*StreamConsumer, *redis.Client, *testutil.MemoryRepository) {
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
	consumer, err := NewStreamConsumer(client, Config{
		StreamName:   stream,
		StreamGroup:  group,
		DLQName:      dlq,
		ConsumerName: consumerName,
		MaxTextBytes: maxTextBytes,
	}, service, NewRetryDLQ(retryMax, backoff))
	require.NoError(t, err)
	require.NoError(t, consumer.ensureGroup(context.Background()))
	return consumer, client, repo
}

// publish añade un evento JSON al stream del consumer. El XADD lleva dos
// fields: document_id como clave del mensaje y payload con el JSON entero
// (delta #13, events.md §Convenciones).
func publish(t *testing.T, client *redis.Client, consumer *StreamConsumer, payload string) {
	t.Helper()
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: consumer.streamName,
		Values: map[string]any{"document_id": documentIDOf(t, payload), "payload": payload},
	}).Result()
	require.NoError(t, err)
}

// documentIDOf extrae el document_id del payload para usarlo como clave del
// mensaje; un JSON ilegible usa el payload entero para no romper el test.
func documentIDOf(t *testing.T, payload string) string {
	t.Helper()
	var body struct {
		DocumentID string `json:"document_id"`
	}
	if err := json.Unmarshal([]byte(payload), &body); err != nil || body.DocumentID == "" {
		return payload
	}
	return body.DocumentID
}

// pendingLen cuenta los mensajes pendientes (sin ACK) del grupo.
func pendingLen(t *testing.T, client *redis.Client, consumer *StreamConsumer) int64 {
	t.Helper()
	pending, err := client.XPending(context.Background(), consumer.streamName, consumer.streamGroup).Result()
	require.NoError(t, err)
	return pending.Count
}

// consumeOnce procesa un lote del stream.
func consumeOnce(t *testing.T, consumer *StreamConsumer) {
	t.Helper()
	require.NoError(t, consumer.consume(context.Background()))
}

// dlqEntries devuelve las entradas de la DLQ del consumer.
func dlqEntries(t *testing.T, client *redis.Client, consumer *StreamConsumer) []redis.XMessage {
	t.Helper()
	entries, err := client.XRange(context.Background(), consumer.dlqName, "-", "+").Result()
	require.NoError(t, err)
	return entries
}

// requireDLQ verifica que el mensaje fallido está en la DLQ con el error esperado.
func requireDLQ(t *testing.T, client *redis.Client, consumer *StreamConsumer, errMsg string) {
	t.Helper()
	entries := dlqEntries(t, client, consumer)
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Values["error"], errMsg)
}

// ingest publica el payload, consume un lote y devuelve el documento persistido.
func ingest(t *testing.T, client *redis.Client, consumer *StreamConsumer, repo *testutil.MemoryRepository, payload string) domain.Document {
	t.Helper()
	publish(t, client, consumer, payload)
	consumeOnce(t, consumer)
	doc, err := repo.FindByDocumentID(context.Background(), documentIDOf(t, payload))
	require.NoError(t, err)
	return *doc
}

const integOriginal = `{
	"event_type": "original",
	"schema_version": 1,
	"document_id": "doc-1",
	"checksum": "chk-1",
	"extracted_text": "texto",
	"extraction_time_ms": 342,
	"mime_type": "application/pdf",
	"summary_requested": true,
	"summary_status": "RESOLVED",
	"filename": "informe.pdf",
	"metadata": {"filename": "x.pdf"}
}`

func TestConsumer_Integration_Original_CreatesPendingAndAcks(t *testing.T) {
	consumer, client, repo := newIntegrationConsumer(t, 5, 50*time.Millisecond, integMaxText, "it-consumer-1")

	doc := ingest(t, client, consumer, repo, integOriginal)

	require.Equal(t, domain.SummaryPending, doc.SummaryStatus)
	require.Equal(t, int64(342), doc.ExtractionTimeMS)
	require.Equal(t, "application/pdf", doc.Metadata.MimeType)
	require.Equal(t, "informe.pdf", doc.Metadata.Filename)
	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_Original_Duplicate_AcksAndDiscards(t *testing.T) {
	consumer, client, repo := newIntegrationConsumer(t, 5, 50*time.Millisecond, integMaxText, "it-consumer-2")

	ingest(t, client, consumer, repo, integOriginal)
	ingest(t, client, consumer, repo, integOriginal)

	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_SummaryResolved_Completes(t *testing.T) {
	consumer, client, repo := newIntegrationConsumer(t, 5, 50*time.Millisecond, integMaxText, "it-consumer-3")

	ingest(t, client, consumer, repo, integOriginal)
	doc := ingest(t, client, consumer, repo, `{
		"event_type": "summary_resolved",
		"schema_version": 1,
		"document_id": "doc-1",
		"summary": "resumen",
		"summary_time_ms": 120
	}`)

	testutil.RequireSummaryResolved(t, doc, "resumen", 120)
	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_OrphanSummary_GoesToDLQ(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 1, 50*time.Millisecond, integMaxText, "it-consumer-4")

	publish(t, client, consumer, `{
		"event_type": "summary_resolved",
		"schema_version": 1,
		"document_id": "no-existe",
		"summary": "huérfano",
		"summary_time_ms": 5
	}`)

	consumeOnce(t, consumer)

	require.Zero(t, pendingLen(t, client, consumer))
	requireDLQ(t, client, consumer, "document not found")
}

func TestConsumer_Integration_InvalidEvent_RetriesThenDLQ(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 2, 100*time.Millisecond, integMaxText, "it-consumer-5")

	publish(t, client, consumer, `{
		"event_type": "original",
		"schema_version": 99,
		"document_id": "doc-1",
		"checksum": "chk-1",
		"extracted_text": "texto",
		"mime_type": "application/pdf",
		"metadata": {"filename": "x.pdf"}
	}`)

	consumeOnce(t, consumer)
	require.Equal(t, int64(1), pendingLen(t, client, consumer))

	time.Sleep(200 * time.Millisecond)
	consumeOnce(t, consumer)

	require.Zero(t, pendingLen(t, client, consumer))
	entries := dlqEntries(t, client, consumer)
	require.Len(t, entries, 1)
	require.Equal(t, "2", entries[0].Values["attempts"])
}

func TestConsumer_Integration_InvalidJSON_GoesToDLQ(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 1, 50*time.Millisecond, integMaxText, "it-consumer-6")

	publish(t, client, consumer, "{no válido")

	consumeOnce(t, consumer)

	require.Zero(t, pendingLen(t, client, consumer))
}

func TestConsumer_Integration_Original_TooLarge_GoesToDLQ(t *testing.T) {
	consumer, client, _ := newIntegrationConsumer(t, 1, 50*time.Millisecond, 4, "it-consumer-7")

	publish(t, client, consumer, `{
		"event_type": "original",
		"schema_version": 1,
		"document_id": "doc-1",
		"checksum": "chk-1",
		"extracted_text": "demasiado texto",
		"mime_type": "application/pdf",
		"metadata": {"filename": "x.pdf"}
	}`)

	consumeOnce(t, consumer)

	require.Zero(t, pendingLen(t, client, consumer))
	requireDLQ(t, client, consumer, "extracted text too large")
}

func TestConsumer_Integration_Run_StopsOnCancel(t *testing.T) {
	consumer, _, _ := newIntegrationConsumer(t, 5, 50*time.Millisecond, integMaxText, "it-consumer-8")

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

// TestConsumer_Integration_TwoConsumers_ShareGroup verifica que dos
// consumers con nombres distintos en el mismo grupo reparten los mensajes
// sin duplicarlos (garantía at-least-once con réplicas horizontales).
func TestConsumer_Integration_TwoConsumers_ShareGroup(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		t.Skip("Redis no disponible para tests de integración")
		return
	}
	t.Cleanup(func() { _ = client.Close() })

	repo := testutil.NewMemoryRepository()
	service := application.NewDocumentService(repo)

	suffix := time.Now().UnixNano()
	stream := fmt.Sprintf("it-events-shared-%d", suffix)
	group := fmt.Sprintf("it-group-shared-%d", suffix)
	dlq := fmt.Sprintf("it-dlq-shared-%d", suffix)

	consumerA, err := NewStreamConsumer(client, Config{
		StreamName:   stream,
		StreamGroup:  group,
		DLQName:      dlq,
		ConsumerName: "replica-A",
		MaxTextBytes: integMaxText,
	}, service, NewRetryDLQ(5, 50*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, consumerA.ensureGroup(context.Background()))

	consumerB, err := NewStreamConsumer(client, Config{
		StreamName:   stream,
		StreamGroup:  group,
		DLQName:      dlq,
		ConsumerName: "replica-B",
		MaxTextBytes: integMaxText,
	}, service, NewRetryDLQ(5, 50*time.Millisecond))
	require.NoError(t, err)

	ctx, cancelConsumers := context.WithCancel(context.Background())
	defer cancelConsumers()

	go func() { _ = consumerA.Run(ctx) }()
	go func() { _ = consumerB.Run(ctx) }()

	// Publicar 4 eventos originales
	for i := 1; i <= 4; i++ {
		publish(t, client, consumerA, fmt.Sprintf(`{
			"event_type": "original",
			"schema_version": 1,
			"document_id": "doc-%d",
			"checksum": "chk-%d",
			"extracted_text": "texto %d",
			"extraction_time_ms": 100,
			"mime_type": "application/pdf",
			"summary_requested": false,
			"summary_status": "NOT_REQUESTED",
			"filename": "file-%d.pdf",
			"metadata": {"filename": "x.pdf"}
		}`, i, i, i, i))
	}

	// Poll hasta que todos los documentos estén en el repo (sin duplicados)
	deadline := time.Now().Add(5 * time.Second)
	for {
		docs, _, err := repo.List(context.Background(), domain.ListFilter{}, 1, 10)
		require.NoError(t, err)
		if len(docs) >= 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout esperando 4 documentos, got %d", len(docs))
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Assert: exactamente 4 docs (sin duplicados) y pending 0
	docs, _, err := repo.List(context.Background(), domain.ListFilter{}, 1, 10)
	require.NoError(t, err)
	require.Len(t, docs, 4)

	require.Zero(t, pendingLen(t, client, consumerA))
	require.Zero(t, pendingLen(t, client, consumerB))
}
