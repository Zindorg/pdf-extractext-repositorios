package mongodb

import (
	"context"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

type CircuitBreakerRepository struct {
	repo *MongoDocumentRepository
	cb   *CircuitBreaker
}

func NewCircuitBreakerRepository(repo *MongoDocumentRepository, cb *CircuitBreaker) *CircuitBreakerRepository {
	return &CircuitBreakerRepository{
		repo: repo,
		cb:   cb,
	}
}

func (cbr *CircuitBreakerRepository) Insert(ctx context.Context, doc domain.Document) error {
	return cbr.cb.Execute(func() error {
		return cbr.repo.Insert(ctx, doc)
	})
}

func (cbr *CircuitBreakerRepository) Complete(ctx context.Context, documentID string, summary string, summaryTimeMS int64, resolvedAt *time.Time) (*domain.Document, error) {
	var result *domain.Document
	err := cbr.cb.Execute(func() error {
		var err error
		result, err = cbr.repo.Complete(ctx, documentID, summary, summaryTimeMS, resolvedAt)
		return err
	})
	return result, err
}

func (cbr *CircuitBreakerRepository) FindByDocumentID(ctx context.Context, documentID string) (*domain.Document, error) {
	var result *domain.Document
	err := cbr.cb.Execute(func() error {
		var err error
		result, err = cbr.repo.FindByDocumentID(ctx, documentID)
		return err
	})
	return result, err
}

func (cbr *CircuitBreakerRepository) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	var result *domain.Document
	err := cbr.cb.Execute(func() error {
		var err error
		result, err = cbr.repo.FindByChecksum(ctx, checksum)
		return err
	})
	return result, err
}

func (cbr *CircuitBreakerRepository) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	var result []domain.Document
	var total int64
	err := cbr.cb.Execute(func() error {
		var err error
		result, total, err = cbr.repo.List(ctx, filter, page, pageSize)
		return err
	})
	return result, total, err
}

func (cbr *CircuitBreakerRepository) SoftDelete(ctx context.Context, documentID string) error {
	return cbr.cb.Execute(func() error {
		return cbr.repo.SoftDelete(ctx, documentID)
	})
}

func (cbr *CircuitBreakerRepository) Restore(ctx context.Context, documentID string) error {
	return cbr.cb.Execute(func() error {
		return cbr.repo.Restore(ctx, documentID)
	})
}

var _ domain.DocumentRepository = (*CircuitBreakerRepository)(nil)
