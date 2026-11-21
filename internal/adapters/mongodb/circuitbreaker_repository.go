package mongodb

import (
	"context"
	"errors"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// businessErrors son respuestas normales del repositorio (la base respondió
// bien): no cuentan como fallo del breaker. Un 404 o un duplicado no indican
// que Mongo esté caído.
var businessErrors = []error{
	domain.ErrNotFound,
	domain.ErrDuplicateChecksum,
	domain.ErrDuplicateDocumentID,
	domain.ErrRestoreConflict,
	errDuplicate,
	context.Canceled, // el cliente abortó: no es culpa de la base
}

func isBusinessError(err error) bool {
	for _, be := range businessErrors {
		if errors.Is(err, be) {
			return true
		}
	}
	return false
}

// CircuitBreakerRepository decora cualquier domain.DocumentRepository con un
// circuit breaker. Los errores de negocio se devuelven tal cual sin contar
// como fallo.
type CircuitBreakerRepository struct {
	repo domain.DocumentRepository
	cb   *CircuitBreaker
}

func NewCircuitBreakerRepository(repo domain.DocumentRepository, cb *CircuitBreaker) *CircuitBreakerRepository {
	return &CircuitBreakerRepository{
		repo: repo,
		cb:   cb,
	}
}

// guard ejecuta fn protegida por el breaker: los errores de negocio se
// registran como éxito para el breaker y se devuelven al llamador.
func guard(cb *CircuitBreaker, fn func() error) error {
	var businessErr error
	err := cb.Execute(func() error {
		e := fn()
		if isBusinessError(e) {
			businessErr = e
			return nil
		}
		return e
	})
	if err != nil {
		return err
	}
	return businessErr
}

// withBreaker es guard para operaciones que devuelven un valor.
func withBreaker[T any](cb *CircuitBreaker, fn func() (T, error)) (T, error) {
	var result T
	err := guard(cb, func() error {
		var e error
		result, e = fn()
		return e
	})
	return result, err
}

func (cbr *CircuitBreakerRepository) Insert(ctx context.Context, doc domain.Document) error {
	return guard(cbr.cb, func() error { return cbr.repo.Insert(ctx, doc) })
}

func (cbr *CircuitBreakerRepository) Complete(ctx context.Context, documentID string, summary string, summaryTimeMS int64, resolvedAt *time.Time) (*domain.Document, error) {
	return withBreaker(cbr.cb, func() (*domain.Document, error) {
		return cbr.repo.Complete(ctx, documentID, summary, summaryTimeMS, resolvedAt)
	})
}

func (cbr *CircuitBreakerRepository) FindByDocumentID(ctx context.Context, documentID string) (*domain.Document, error) {
	return withBreaker(cbr.cb, func() (*domain.Document, error) {
		return cbr.repo.FindByDocumentID(ctx, documentID)
	})
}

func (cbr *CircuitBreakerRepository) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	return withBreaker(cbr.cb, func() (*domain.Document, error) {
		return cbr.repo.FindByChecksum(ctx, checksum)
	})
}

func (cbr *CircuitBreakerRepository) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	var total int64
	docs, err := withBreaker(cbr.cb, func() ([]domain.Document, error) {
		var (
			list []domain.Document
			e    error
		)
		list, total, e = cbr.repo.List(ctx, filter, page, pageSize)
		return list, e
	})
	return docs, total, err
}

func (cbr *CircuitBreakerRepository) SoftDelete(ctx context.Context, documentID string) error {
	return guard(cbr.cb, func() error { return cbr.repo.SoftDelete(ctx, documentID) })
}

func (cbr *CircuitBreakerRepository) Restore(ctx context.Context, documentID string) error {
	return guard(cbr.cb, func() error { return cbr.repo.Restore(ctx, documentID) })
}

var _ domain.DocumentRepository = (*CircuitBreakerRepository)(nil)
