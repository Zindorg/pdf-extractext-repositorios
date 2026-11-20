package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/redis/go-redis/v9"
)

const (
	keyByIDPrefix       = "doc:"
	keyByChecksumPrefix = "docchk:"
)

// CachedRepository envuelve un domain.DocumentRepository y añade
// read-through cache con invalidación en escrituras.
// Fail-open: errores de Redis se loguean y delegan al repo subyacente.
type CachedRepository struct {
	repo   domain.DocumentRepository
	client redis.Cmdable
	ttl    time.Duration
}

// NewCachedRepository crea el decorator. ttl == 0 deshabilita el cache
// (pass-through sin overhead).
func NewCachedRepository(repo domain.DocumentRepository, client redis.Cmdable, ttl time.Duration) *CachedRepository {
	if ttl == 0 {
		return &CachedRepository{repo: repo, client: nil, ttl: 0}
	}
	return &CachedRepository{repo: repo, client: client, ttl: ttl}
}

func (c *CachedRepository) Insert(ctx context.Context, doc domain.Document) error {
	return c.repo.Insert(ctx, doc)
}

func (c *CachedRepository) Complete(ctx context.Context, documentID string, summary string, summaryTimeMS int64, resolvedAt *time.Time) (*domain.Document, error) {
	doc, err := c.repo.Complete(ctx, documentID, summary, summaryTimeMS, resolvedAt)
	if err == nil && c.client != nil {
		c.invalidate(ctx, documentID, doc.Checksum)
	}
	return doc, err
}

func (c *CachedRepository) FindByDocumentID(ctx context.Context, documentID string) (*domain.Document, error) {
	return c.readThrough(ctx, keyByIDPrefix+documentID, func() (*domain.Document, error) {
		return c.repo.FindByDocumentID(ctx, documentID)
	})
}

func (c *CachedRepository) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	return c.readThrough(ctx, keyByChecksumPrefix+checksum, func() (*domain.Document, error) {
		return c.repo.FindByChecksum(ctx, checksum)
	})
}

// readThrough resuelve una lectura: hit → devuelve lo cacheado; miss → carga
// del repo subyacente y puebla el cache con el TTL configurado.
// Los errores (incluido ErrNotFound) no se cachean.
func (c *CachedRepository) readThrough(ctx context.Context, key string, load func() (*domain.Document, error)) (*domain.Document, error) {
	if c.client == nil {
		return load()
	}
	if doc := c.get(ctx, key); doc != nil {
		return doc, nil
	}
	doc, err := load()
	if err != nil {
		return nil, err
	}
	c.set(ctx, key, doc)
	return doc, nil
}

// get devuelve el documento cacheado o nil ante miss, error de Redis o
// entrada corrupta (fail-open: el caller siempre puede ir al repo).
func (c *CachedRepository) get(ctx context.Context, key string) *domain.Document {
	val, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		slog.Warn("cache get failed", "key", key, "error", err)
		return nil
	}
	var doc domain.Document
	if err := json.Unmarshal(val, &doc); err != nil {
		slog.Warn("cache decode failed", "key", key, "error", err)
		return nil
	}
	return &doc
}

// set guarda el documento con TTL; un fallo se loguea y no afecta la respuesta.
func (c *CachedRepository) set(ctx context.Context, key string, doc *domain.Document) {
	payload, err := json.Marshal(doc)
	if err != nil {
		slog.Warn("cache encode failed", "key", key, "error", err)
		return
	}
	if err := c.client.Set(ctx, key, payload, c.ttl).Err(); err != nil {
		slog.Warn("cache set failed", "key", key, "error", err)
	}
}

func (c *CachedRepository) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	return c.repo.List(ctx, filter, page, pageSize)
}

func (c *CachedRepository) SoftDelete(ctx context.Context, documentID string) error {
	err := c.repo.SoftDelete(ctx, documentID)
	if err == nil && c.client != nil {
		c.invalidateByLookup(ctx, documentID)
	}
	return err
}

func (c *CachedRepository) Restore(ctx context.Context, documentID string) error {
	err := c.repo.Restore(ctx, documentID)
	if err == nil && c.client != nil {
		c.invalidateByLookup(ctx, documentID)
	}
	return err
}

// invalidateByLookup necesita el checksum para borrar también docchk:{checksum};
// lo lee del repo subyacente (best-effort: sin checksum invalida solo doc:{id}).
func (c *CachedRepository) invalidateByLookup(ctx context.Context, documentID string) {
	checksum := ""
	if doc, _ := c.repo.FindByDocumentID(ctx, documentID); doc != nil {
		checksum = doc.Checksum
	}
	c.invalidate(ctx, documentID, checksum)
}

func (c *CachedRepository) invalidate(ctx context.Context, documentID, checksum string) {
	if c.client == nil {
		return
	}
	keys := []string{keyByIDPrefix + documentID}
	if checksum != "" {
		keys = append(keys, keyByChecksumPrefix+checksum)
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("cache invalidate failed", "keys", keys, "error", err)
	}
}

var _ domain.DocumentRepository = (*CachedRepository)(nil)