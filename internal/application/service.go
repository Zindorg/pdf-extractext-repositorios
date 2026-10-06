package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

var ErrNotImplemented = errors.New("not implemented yet")

// DocumentService orquesta los casos de uso del microservicio.
// En esta fase es un esqueleto: las firmas son definitivas y el cuerpo
// se completa en las fases de lógica (dedup, soft-delete, restore, list).
type DocumentService struct {
	repo domain.DocumentRepository
}

func NewDocumentService(repo domain.DocumentRepository) *DocumentService {
	return &DocumentService{repo: repo}
}

func unimplemented(what string) error {
	return fmt.Errorf("%s: %w", what, ErrNotImplemented)
}

func (s *DocumentService) CreatePending(ctx context.Context, doc domain.Document) (*domain.Document, error) {
	now := time.Now().UTC()
	doc.Status = domain.StatusPending
	doc.CreatedAt = now
	doc.UpdatedAt = now

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}
	return s.repo.FindByDocumentID(ctx, doc.DocumentID)
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
	return nil, unimplemented("get")
}

func (s *DocumentService) GetByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	return nil, unimplemented("findByChecksum")
}

func (s *DocumentService) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	return nil, 0, unimplemented("list")
}

func (s *DocumentService) SoftDelete(ctx context.Context, documentID string) error {
	return unimplemented("softDelete")
}

func (s *DocumentService) Restore(ctx context.Context, documentID string) error {
	return unimplemented("restore")
}
