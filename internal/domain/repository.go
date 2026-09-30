package domain

import (
	"context"
	"time"
)

type ListFilter struct {
	Status         *Status
	Filename       *string
	IncludeDeleted bool
	CreatedFrom    *time.Time // nil → sin límite inferior
	CreatedTo      *time.Time // nil → sin límite superior
}

type DocumentRepository interface {
	Insert(ctx context.Context, doc Document) error
	UpdateStatus(ctx context.Context, documentID string, status Status, summary *string) (*Document, error)
	FindByDocumentID(ctx context.Context, documentID string) (*Document, error)
	FindByChecksum(ctx context.Context, checksum string) (*Document, error)
	List(ctx context.Context, filter ListFilter, page int, pageSize int) ([]Document, int64, error)
	SoftDelete(ctx context.Context, documentID string) error
	Restore(ctx context.Context, documentID string) error
}
