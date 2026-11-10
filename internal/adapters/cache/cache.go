package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/redis/go-redis/v9"
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
	if c.client == nil {
		return c.repo.FindByDocumentID(ctx, documentID)
	}
	key := "doc:" + documentID
	val, err := c.client.Get(ctx, key).Bytes()
	if err == nil {
		var doc domain.Document
		if json.Unmarshal(val, &doc) == nil {
			return &doc, nil
		}
	} else if err != redis.Nil {
		slog.Warn("cache get failed", "key", key, "error", err)
	}
	return c.repo.FindByDocumentID(ctx, documentID)
}

func (c *CachedRepository) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	if c.client == nil {
		return c.repo.FindByChecksum(ctx, checksum)
	}
	key := "docchk:" + checksum
	val, err := c.client.Get(ctx, key).Bytes()
	if err == nil {
		var doc domain.Document
		if json.Unmarshal(val, &doc) == nil {
			return &doc, nil
		}
	} else if err != redis.Nil {
		slog.Warn("cache get failed", "key", key, "error", err)
	}
	return c.repo.FindByChecksum(ctx, checksum)
}

func (c *CachedRepository) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	return c.repo.List(ctx, filter, page, pageSize)
}

func (c *CachedRepository) SoftDelete(ctx context.Context, documentID string) error {
	err := c.repo.SoftDelete(ctx, documentID)
	if err == nil && c.client != nil {
		// Para invalidar necesitamos el checksum; hacemos best-effort:
		// leemos del repo antes de borrar (ya tenemos doc si no falló).
		if doc, _ := c.repo.FindByDocumentID(ctx, documentID); doc != nil {
			c.invalidate(ctx, documentID, doc.Checksum)
		} else {
			c.invalidate(ctx, documentID, "")
		}
	}
	return err
}

func (c *CachedRepository) Restore(ctx context.Context, documentID string) error {
	err := c.repo.Restore(ctx, documentID)
	if err == nil && c.client != nil {
		if doc, _ := c.repo.FindByDocumentID(ctx, documentID); doc != nil {
			c.invalidate(ctx, documentID, doc.Checksum)
		} else {
			c.invalidate(ctx, documentID, "")
		}
	}
	return err
}

func (c *CachedRepository) invalidate(ctx context.Context, documentID, checksum string) {
	if c.client == nil {
		return
	}
	keys := []string{"doc:" + documentID}
	if checksum != "" {
		keys = append(keys, "docchk:"+checksum)
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("cache invalidate failed", "keys", keys, "error", err)
	}
}

var _ domain.DocumentRepository = (*CachedRepository)(nil)
