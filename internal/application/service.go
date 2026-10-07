package application

import (
	"context"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// DocumentService orquesta los casos de uso del microservicio.
// CreatePending, CompleteWithSummary, Get*, List, SoftDelete y Restore
// están implementados.
type DocumentService struct {
	repo domain.DocumentRepository
}

func NewDocumentService(repo domain.DocumentRepository) *DocumentService {
	return &DocumentService{repo: repo}
}

// notFoundIfDeleted aplica la regla única de visibilidad del servicio:
// un documento soft-deleted responde igual que si no existiera.
func notFoundIfDeleted(doc *domain.Document, err error) (*domain.Document, error) {
	if err != nil {
		return nil, err
	}
	if doc.IsDeleted() {
		return nil, domain.ErrNotFound
	}
	return doc, nil
}

func (s *DocumentService) CreatePending(ctx context.Context, doc domain.Document) (*domain.Document, error) {
	now := domain.Now()
	doc.Status = domain.StatusPending
	doc.CreatedAt = now
	doc.UpdatedAt = now

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *DocumentService) CompleteWithSummary(ctx context.Context, documentID, summary string) (*domain.Document, error) {
	current, err := s.repo.FindByDocumentID(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if current.IsCompleted() {
		return current, nil
	}
	sum := summary
	return s.repo.UpdateStatus(ctx, documentID, domain.StatusCompleted, &sum)
}

func (s *DocumentService) GetByDocumentID(ctx context.Context, documentID string) (*domain.Document, error) {
	return notFoundIfDeleted(s.repo.FindByDocumentID(ctx, documentID))
}

func (s *DocumentService) GetByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	return notFoundIfDeleted(s.repo.FindByChecksum(ctx, checksum))
}

func (s *DocumentService) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = domain.DefaultPageSize
	}
	if pageSize > domain.MaxPageSize {
		pageSize = domain.MaxPageSize
	}
	return s.repo.List(ctx, filter, page, pageSize)
}

func (s *DocumentService) SoftDelete(ctx context.Context, documentID string) error {
	return s.repo.SoftDelete(ctx, documentID)
}

func (s *DocumentService) Restore(ctx context.Context, documentID string) error {
	return s.repo.Restore(ctx, documentID)
}
