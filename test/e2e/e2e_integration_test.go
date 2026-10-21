//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/marco/pdf-extractext-repositorios/internal/adapters/health"
	mongodb "github.com/marco/pdf-extractext-repositorios/internal/adapters/mongodb"
	redisadapter "github.com/marco/pdf-extractext-repositorios/internal/adapters/redis"
	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/config"
)

const (
	e2eRedisAddr = "localhost:6379"
	e2eMongoURI  = "mongodb://localhost:27017/text_extractor_db_e2e"
	e2eMongoDB   = "text_extractor_db_e2e"
)

func setupE2E(t *testing.T) (*httptest.Server, *redis.Client, *mongo.Client, string) {
	t.Helper()

	cfg := &config.Config{
		HTTPPort:        "8083",
		MongoDBURI:      e2eMongoURI,
		MongoDBDatabase: e2eMongoDB,
		RedisAddr:       e2eRedisAddr,
		StreamName:      "document-events-e2e",
		StreamGroup:     "documents-persister-e2e",
		DLQName:         "document-events-e2e-dlq",
		RetryMax:        3,
		RetryBackoff:    50 * time.Millisecond,
		MaxTextBytes:    15 << 20,
		LogLevel:        "info",
	}

	// Redis
	redisCli := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	require.NoError(t, redisCli.Ping(context.Background()).Err())
	_ = redisCli.Del(context.Background(), cfg.StreamName, cfg.DLQName).Err()
	_ = redisCli.XGroupDestroy(context.Background(), cfg.StreamName, cfg.StreamGroup).Err()
	require.NoError(t, redisCli.XGroupCreateMkStream(context.Background(), cfg.StreamName, cfg.StreamGroup, "$").Err())

	// Mongo
	mongoCli, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoDBURI))
	require.NoError(t, err)
	require.NoError(t, mongoCli.Ping(context.Background(), readpref.Primary()))
	require.NoError(t, mongoCli.Database(cfg.MongoDBDatabase).Drop(context.Background()))

	// Infra
	repo := mongodb.NewMongoDocumentRepository(mongoCli.Database(cfg.MongoDBDatabase))
	require.NoError(t, repo.EnsureIndexes(context.Background()))

	service := application.NewDocumentService(repo)
	retry := redisadapter.NewRetryDLQ(cfg.RetryMax, cfg.RetryBackoff)
	consumer, err := redisadapter.NewStreamConsumer(redisCli, cfg.StreamName, cfg.StreamGroup, cfg.DLQName, service, retry, cfg.MaxTextBytes)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = consumer.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		_ = redisCli.Close()
		_ = mongoCli.Disconnect(context.Background())
	})

	// HTTP server
	handler := api.NewDocumentHandler(service, health.NewPinger(mongoCli, redisCli))
	router := api.NewRouter(handler)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return srv, redisCli, mongoCli, cfg.StreamName
}

func publishEvent(t *testing.T, redisCli *redis.Client, stream string, event map[string]any) {
	t.Helper()
	b, err := json.Marshal(event)
	require.NoError(t, err)
	res := redisCli.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"document_id": event["document_id"], "payload": string(b)},
	})
	require.NoError(t, res.Err())
}

// documentStatus lee el status del documento; false si aún no responde 200.
func documentStatus(t *testing.T, client *http.Client, url string) (string, bool) {
	t.Helper()
	resp, err := client.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var doc struct {
		SummaryStatus string `json:"summary_status"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&doc))
	return doc.SummaryStatus, true
}

// waitForStatus reintenta el GET hasta que el documento responde con el
// status esperado o se agota el tiempo.
func waitForStatus(t *testing.T, client *http.Client, url, status string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, ok := documentStatus(t, client, url)
		if ok && got == status {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("esperaba status %q, último %q", status, got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestE2E_Flow_OriginalThenSummaryThenOperations(t *testing.T) {
	// Skip if no Redis/Mongo running in local env
	if os.Getenv("SKIP_E2E") != "" {
		t.Skip("skipping e2e")
	}

	srv, redisCli, mongoCli, stream := setupE2E(t)
	client := srv.Client()

	// 1. Health
	resp, err := client.Get(srv.URL + "/api/v1/health")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	docID := "e2e-doc-1"
	checksum := "chk-e2e-1"

	// 2. Publish original
	original := map[string]any{
		"event_type":         "original",
		"schema_version":     1,
		"document_id":        docID,
		"checksum":           checksum,
		"extracted_text":     "texto extraido",
		"extraction_time_ms": 342,
		"mime_type":          "application/pdf",
		"summary_requested":  true,
		"filename":           "informe-e2e.pdf",
		"metadata":           map[string]any{"filename": "test.pdf", "size_bytes": 10, "page_count": 1},
	}
	publishEvent(t, redisCli, stream, original)
	waitForStatus(t, client, srv.URL+"/api/v1/documents/"+docID, "PENDING")

	// 3. GET by id - pending
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	resp.Body.Close()
	require.Equal(t, "PENDING", got["summary_status"])
	require.Equal(t, "AVAILABLE", got["delivery_status"])
	require.Equal(t, "informe-e2e.pdf", got["filename"])
	require.Equal(t, "application/pdf", got["mime_type"])
	require.Equal(t, "informe-e2e.pdf", got["metadata"].(map[string]any)["filename"])

	// 4. GET by checksum
	resp, err = client.Get(srv.URL + "/api/v1/documents/checksum/" + checksum)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 5. Download original
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID + "/download/original")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	require.Equal(t, "texto extraido", buf.String())
	resp.Body.Close()

	// 6. Download summary - 409 summary_pending
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID + "/download/summary")
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	var errBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	resp.Body.Close()
	require.Equal(t, "summary_pending", errBody["code"].(string))

	// 7. Publish summary_resolved
	summary := map[string]any{
		"event_type":      "summary_resolved",
		"schema_version":  1,
		"document_id":     docID,
		"summary":         "resumen",
		"summary_time_ms": 120,
	}
	publishEvent(t, redisCli, stream, summary)
	waitForStatus(t, client, srv.URL+"/api/v1/documents/"+docID, "RESOLVED")

	// 8. GET by id - completed with processing time
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	resp.Body.Close()
	require.Equal(t, "RESOLVED", got["summary_status"])
	require.NotNil(t, got["summary"])
	require.Equal(t, float64(462), got["processing_time_ms"].(float64)) // 342+120

	// 9. Download summary
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID + "/download/summary")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	buf.Reset()
	_, _ = buf.ReadFrom(resp.Body)
	require.Equal(t, "resumen", buf.String())
	resp.Body.Close()

	// 10. List
	resp, err = client.Get(srv.URL + "/api/v1/documents")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	resp.Body.Close()
	items := list["items"].([]any)
	require.NotEmpty(t, items)

	// 11. Soft delete
	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/documents/"+docID, nil)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// 12. GET returns 404
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// 13. Download original returns 404
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID + "/download/original")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// 14. Restore
	req, err = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/documents/"+docID+"/restore", nil)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// 15. GET returns 200
	resp, err = client.Get(srv.URL + "/api/v1/documents/" + docID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Cleanup
	_ = mongoCli.Database(e2eMongoDB).Drop(context.Background())
}
