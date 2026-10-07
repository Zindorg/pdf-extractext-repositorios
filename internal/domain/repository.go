package domain

import (
	"context"
	"time"
)

// Pagination del listado: los defaults viven acá; la normalización de
// page/pageSize es responsabilidad del servicio, no del repositorio.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

type ListFilter struct {
	Status         *Status
	Filename       *string
	IncludeDeleted bool
	CreatedFrom    *time.Time // nil → sin límite inferior
	CreatedTo      *time.Time // nil → sin límite superior
}

// DocumentRepository es el puerto de persistencia de documentos.
// Precondiciones de List: page >= 1 y 1 <= pageSize <= MaxPageSize
// (el servicio normaliza antes de llamar).
type DocumentRepository interface {
	Insert(ctx context.Context, doc Document) error
	UpdateStatus(ctx context.Context, documentID string, status Status, summary *string, summaryTimeMS int64) (*Document, error)
	FindByDocumentID(ctx context.Context, documentID string) (*Document, error)
	FindByChecksum(ctx context.Context, checksum string) (*Document, error)
	List(ctx context.Context, filter ListFilter, page int, pageSize int) ([]Document, int64, error)
	SoftDelete(ctx context.Context, documentID string) error
	Restore(ctx context.Context, documentID string) error
}
