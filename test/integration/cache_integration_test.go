//go:build integration

package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/marco/pdf-extractext-repositorios/internal/adapters/cache"
	"github.com/marco/pdf-extractext-repositorios/internal/adapters/mongodb"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

const (
	intRedisAddr = "localhost:6379"
	intMongoURI  = "mongodb://localhost:27017/text_extractor_db_cache_int"
	intMongoDB   = "text_extractor_db_cache_int"
)

func setupCacheIntegration(t *testing.T) (*redis.Client, *mongo.Client, domain.DocumentRepository) {
	t.Helper()

	// Redis
	redisCli := redis.NewClient(&redis.Options{Addr: intRedisAddr})
	require.NoError(t, redisCli.Ping(context.Background()).Err())
	t.Cleanup(func() { _ = redisCli.Close() })

	// Mongo
	mongoCli, err := mongo.Connect(options.Client().ApplyURI(intMongoURI))
	require.NoError(t, err)
	require.NoError(t, mongoCli.Ping(context.Background(), readpref.Primary()))
	require.NoError(t, mongoCli.Database(intMongoDB).Drop(context.Background()))
	t.Cleanup(func() { _ = mongoCli.Disconnect(context.Background()) })

	// Repo chain: Mongo -> CB -> Cache
	baseRepo := mongodb.NewMongoDocumentRepository(mongoCli.Database(intMongoDB))
	require.NoError(t, baseRepo.EnsureIndexes(context.Background()))

	cb := mongodb.NewCircuitBreaker(5, 2, 30*time.Second)
	cbRepo := mongodb.NewCircuitBreakerRepository(baseRepo, cb)
	cachedRepo := cache.NewCachedRepository(cbRepo, redisCli, 10*time.Second)

	return redisCli, mongoCli, cachedRepo
}

func newTestDocument(id, checksum string) domain.Document {
	return domain.Document{
		DocumentID:     id,
		Checksum:       checksum,
		SummaryStatus:  domain.SummaryNotRequested,
		ExtractedText:  "test text",
		ExtractionTimeMS: 100,
		Metadata: domain.Metadata{
			Filename:  "test.pdf",
			MimeType:  "application/pdf",
			SizeBytes: 100,
			PageCount: 1,
		},
	}
}

func TestCacheIntegration_FindByDocumentID_CacheHit(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("skipping integration")
	}
	_, _, repo := setupCacheIntegration(t)

	doc := newTestDocument("cache-int-1", "chk-int-1")
	require.NoError(t, repo.Insert(context.Background(), doc))

	// Primera lectura: miss -> Mongo -> cache
	got, err := repo.FindByDocumentID(context.Background(), "cache-int-1")
	require.NoError(t, err)
	require.Equal(t, "cache-int-1", got.DocumentID)

	// Segunda lectura: hit -> cache (verificamos que no falla y devuelve mismo dato)
	got, err = repo.FindByDocumentID(context.Background(), "cache-int-1")
	require.NoError(t, err)
	require.Equal(t, "cache-int-1", got.DocumentID)
}

func TestCacheIntegration_FindByChecksum_CacheHit(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("skipping integration")
	}
	_, _, repo := setupCacheIntegration(t)

	doc := newTestDocument("cache-int-2", "chk-int-2")
	require.NoError(t, repo.Insert(context.Background(), doc))

	got, err := repo.FindByChecksum(context.Background(), "chk-int-2")
	require.NoError(t, err)
	require.Equal(t, "chk-int-2", got.Checksum)

	got, err = repo.FindByChecksum(context.Background(), "chk-int-2")
	require.NoError(t, err)
	require.Equal(t, "chk-int-2", got.Checksum)
}

func TestCacheIntegration_Complete_InvalidatesCache(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("skipping integration")
	}
	redisCli, _, repo := setupCacheIntegration(t)

	doc := newTestDocument("cache-int-3", "chk-int-3")
	doc.SummaryStatus = domain.SummaryPending
	require.NoError(t, repo.Insert(context.Background(), doc))

	// Poblar cache manualmente (simula primera lectura)
	require.NoError(t, redisCli.Set(context.Background(), "doc:cache-int-3", `{"document_id":"cache-int-3","checksum":"chk-int-3","summary_status":"PENDING"}`, time.Hour).Err())
	require.NoError(t, redisCli.Set(context.Background(), "docchk:chk-int-3", `{"document_id":"cache-int-3","checksum":"chk-int-3","summary_status":"PENDING"}`, time.Hour).Err())

	// Complete invalida cache
	summary := "resumen"
	got, err := repo.Complete(context.Background(), "cache-int-3", summary, 100)
	require.NoError(t, err)
	require.Equal(t, domain.SummaryResolved, got.SummaryStatus)

	// Verificar que cache fue invalidado
	_, err = redisCli.Get(context.Background(), "doc:cache-int-3").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = redisCli.Get(context.Background(), "docchk:chk-int-3").Result()
	require.ErrorIs(t, err, redis.Nil)

	// Lectura posterior -> Mongo -> cache repoblado
	got, err = repo.FindByDocumentID(context.Background(), "cache-int-3")
	require.NoError(t, err)
	require.Equal(t, domain.SummaryResolved, got.SummaryStatus)
}

func TestCacheIntegration_SoftDelete_InvalidatesCache(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("skipping integration")
	}
	redisCli, _, repo := setupCacheIntegration(t)

	doc := newTestDocument("cache-int-4", "chk-int-4")
	doc.SummaryStatus = domain.SummaryResolved
	require.NoError(t, repo.Insert(context.Background(), doc))

	require.NoError(t, redisCli.Set(context.Background(), "doc:cache-int-4", `{"document_id":"cache-int-4","checksum":"chk-int-4","summary_status":"RESOLVED"}`, time.Hour).Err())
	require.NoError(t, redisCli.Set(context.Background(), "docchk:chk-int-4", `{"document_id":"cache-int-4","checksum":"chk-int-4","summary_status":"RESOLVED"}`, time.Hour).Err())

	err := repo.SoftDelete(context.Background(), "cache-int-4")
	require.NoError(t, err)

	_, err = redisCli.Get(context.Background(), "doc:cache-int-4").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = redisCli.Get(context.Background(), "docchk:chk-int-4").Result()
	require.ErrorIs(t, err, redis.Nil)
}

func TestCacheIntegration_Restore_InvalidatesCache(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("skipping integration")
	}
	redisCli, _, repo := setupCacheIntegration(t)

	doc := newTestDocument("cache-int-5", "chk-int-5")
	doc.SummaryStatus = domain.SummaryResolved
	require.NoError(t, repo.Insert(context.Background(), doc))
	require.NoError(t, repo.SoftDelete(context.Background(), "cache-int-5"))

	require.NoError(t, redisCli.Set(context.Background(), "doc:cache-int-5", `{"document_id":"cache-int-5","checksum":"chk-int-5","summary_status":"RESOLVED"}`, time.Hour).Err())
	require.NoError(t, redisCli.Set(context.Background(), "docchk:chk-int-5", `{"document_id":"cache-int-5","checksum":"chk-int-5","summary_status":"RESOLVED"}`, time.Hour).Err())

	err := repo.Restore(context.Background(), "cache-int-5")
	require.NoError(t, err)

	_, err = redisCli.Get(context.Background(), "doc:cache-int-5").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = redisCli.Get(context.Background(), "docchk:chk-int-5").Result()
	require.ErrorIs(t, err, redis.Nil)

	// Verificar que el documento está activo
	got, err := repo.FindByDocumentID(context.Background(), "cache-int-5")
	require.NoError(t, err)
	require.False(t, got.IsDeleted())
}

func TestCacheIntegration_List_NotCached(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("skipping integration")
	}
	_, _, repo := setupCacheIntegration(t)

	doc := newTestDocument("cache-int-6", "chk-int-6")
	require.NoError(t, repo.Insert(context.Background(), doc))

	// List no usa cache; debería funcionar sin poblar Redis
	results, total, err := repo.List(context.Background(), domain.ListFilter{}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, results, 1)
}

func TestCacheIntegration_FailOpen_RedisDown(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") != "" {
		t.Skip("skipping integration")
	}
	redisCli, _, repo := setupCacheIntegration(t)

	doc := newTestDocument("cache-int-7", "chk-int-7")
	require.NoError(t, repo.Insert(context.Background(), doc))

	// Cerrar Redis para simular caída
	redisCli.Close()

	// Fail-open: debe delegar a Mongo sin fallar
	got, err := repo.FindByDocumentID(context.Background(), "cache-int-7")
	require.NoError(t, err)
	require.Equal(t, "cache-int-7", got.DocumentID)

	got, err = repo.FindByChecksum(context.Background(), "chk-int-7")
	require.NoError(t, err)
	require.Equal(t, "chk-int-7", got.Checksum)

	// Write también fail-open
	err = repo.SoftDelete(context.Background(), "cache-int-7")
	require.NoError(t, err)
}