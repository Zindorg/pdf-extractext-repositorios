package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	s := miniredis.RunT(t)
	t.Cleanup(func() { s.Close() })
	return redis.NewClient(&redis.Options{Addr: s.Addr()})
}

func TestCachedRepository_FindByDocumentID_CacheHit(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-1",
		Checksum:      "chk-1",
		SummaryStatus: domain.SummaryNotRequested,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	// Pre-poblar cache
	require.NoError(t, redisClient.Set(context.Background(), "doc:doc-1", `{"document_id":"doc-1","checksum":"chk-1","summary_status":"NOT_REQUESTED"}`, time.Hour).Err())

	result, err := cached.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, "doc-1", result.DocumentID)
}

func TestCachedRepository_FindByDocumentID_CacheMiss_FallsBack(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-2",
		Checksum:      "chk-2",
		SummaryStatus: domain.SummaryNotRequested,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	result, err := cached.FindByDocumentID(context.Background(), "doc-2")
	require.NoError(t, err)
	require.Equal(t, "doc-2", result.DocumentID)
}

func TestCachedRepository_FindByChecksum_CacheHit(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-3",
		Checksum:      "chk-3",
		SummaryStatus: domain.SummaryNotRequested,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	require.NoError(t, redisClient.Set(context.Background(), "docchk:chk-3", `{"document_id":"doc-3","checksum":"chk-3","summary_status":"NOT_REQUESTED"}`, time.Hour).Err())

	result, err := cached.FindByChecksum(context.Background(), "chk-3")
	require.NoError(t, err)
	require.Equal(t, "chk-3", result.Checksum)
}

func TestCachedRepository_FindByChecksum_CacheMiss_FallsBack(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-4",
		Checksum:      "chk-4",
		SummaryStatus: domain.SummaryNotRequested,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	result, err := cached.FindByChecksum(context.Background(), "chk-4")
	require.NoError(t, err)
	require.Equal(t, "chk-4", result.Checksum)
}

func TestCachedRepository_Complete_InvalidatesCache(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-5",
		Checksum:      "chk-5",
		SummaryStatus: domain.SummaryPending,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	// Pre-poblar cache
	require.NoError(t, redisClient.Set(context.Background(), "doc:doc-5", `{"document_id":"doc-5","checksum":"chk-5","summary_status":"PENDING"}`, time.Hour).Err())
	require.NoError(t, redisClient.Set(context.Background(), "docchk:chk-5", `{"document_id":"doc-5","checksum":"chk-5","summary_status":"PENDING"}`, time.Hour).Err())

	summary := "resumen"
	now := time.Now().UTC()
	result, err := cached.Complete(context.Background(), "doc-5", summary, 100, &now)
	require.NoError(t, err)
	require.Equal(t, domain.SummaryResolved, result.SummaryStatus)

	// Cache debe estar invalidado
	_, err = redisClient.Get(context.Background(), "doc:doc-5").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = redisClient.Get(context.Background(), "docchk:chk-5").Result()
	require.ErrorIs(t, err, redis.Nil)
}

func TestCachedRepository_SoftDelete_InvalidatesCache(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-6",
		Checksum:      "chk-6",
		SummaryStatus: domain.SummaryResolved,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	require.NoError(t, redisClient.Set(context.Background(), "doc:doc-6", `{"document_id":"doc-6","checksum":"chk-6","summary_status":"RESOLVED"}`, time.Hour).Err())
	require.NoError(t, redisClient.Set(context.Background(), "docchk:chk-6", `{"document_id":"doc-6","checksum":"chk-6","summary_status":"RESOLVED"}`, time.Hour).Err())

	err := cached.SoftDelete(context.Background(), "doc-6")
	require.NoError(t, err)

	_, err = redisClient.Get(context.Background(), "doc:doc-6").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = redisClient.Get(context.Background(), "docchk:chk-6").Result()
	require.ErrorIs(t, err, redis.Nil)
}

func TestCachedRepository_Restore_InvalidatesCache(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-7",
		Checksum:      "chk-7",
		SummaryStatus: domain.SummaryResolved,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))
	require.NoError(t, repo.SoftDelete(context.Background(), "doc-7"))

	require.NoError(t, redisClient.Set(context.Background(), "doc:doc-7", `{"document_id":"doc-7","checksum":"chk-7","summary_status":"RESOLVED"}`, time.Hour).Err())
	require.NoError(t, redisClient.Set(context.Background(), "docchk:chk-7", `{"document_id":"doc-7","checksum":"chk-7","summary_status":"RESOLVED"}`, time.Hour).Err())

	err := cached.Restore(context.Background(), "doc-7")
	require.NoError(t, err)

	_, err = redisClient.Get(context.Background(), "doc:doc-7").Result()
	require.ErrorIs(t, err, redis.Nil)
	_, err = redisClient.Get(context.Background(), "docchk:chk-7").Result()
	require.ErrorIs(t, err, redis.Nil)
}

func TestCachedRepository_TTLZero_DisablesCache(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	cached := NewCachedRepository(repo, nil, 0) // ttl=0 = disabled

	doc := domain.Document{
		DocumentID:    "doc-8",
		Checksum:      "chk-8",
		SummaryStatus: domain.SummaryNotRequested,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	result, err := cached.FindByDocumentID(context.Background(), "doc-8")
	require.NoError(t, err)
	require.Equal(t, "doc-8", result.DocumentID)

	// Sin client, no debería panicar
	_, err = cached.FindByChecksum(context.Background(), "chk-8")
	require.NoError(t, err)
}

func TestCachedRepository_List_NotCached(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-9",
		Checksum:      "chk-9",
		SummaryStatus: domain.SummaryNotRequested,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	// List no usa cache; si no falla, pasa
	results, total, err := cached.List(context.Background(), domain.ListFilter{}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, results, 1)
}

func TestCachedRepository_FailOpen_GetError(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	// Cerramos el servidor miniredis para simular error de conexión
	redisClient.Close()
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-10",
		Checksum:      "chk-10",
		SummaryStatus: domain.SummaryNotRequested,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	// Error en Redis -> fail-open, delega al repo
	result, err := cached.FindByDocumentID(context.Background(), "doc-10")
	require.NoError(t, err)
	require.Equal(t, "doc-10", result.DocumentID)
}

func TestCachedRepository_FailOpen_DelError(t *testing.T) {
	repo := testutil.NewMemoryRepository()
	redisClient := newTestRedis(t)
	redisClient.Close()
	cached := NewCachedRepository(repo, redisClient, time.Hour)

	doc := domain.Document{
		DocumentID:    "doc-11",
		Checksum:      "chk-11",
		SummaryStatus: domain.SummaryResolved,
	}
	require.NoError(t, repo.Insert(context.Background(), doc))

	// Error en Del -> fail-open, write succeeds, log warn (no falla)
	err := cached.SoftDelete(context.Background(), "doc-11")
	require.NoError(t, err)
}
