package mongodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_ClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "closed", cb.State())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.Execute(func() error { return errors.New("fail") })
	}

	require.Equal(t, "open", cb.State())

	err := cb.Execute(func() error { return nil })
	require.ErrorIs(t, err, ErrCircuitOpen)
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	cb.Execute(func() error { return errors.New("fail") })
	cb.Execute(func() error { return errors.New("fail") })
	require.Equal(t, "open", cb.State())

	time.Sleep(60 * time.Millisecond)

	// State transitions to half-open on next Execute call
	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "half-open", cb.State())

	err = cb.Execute(func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "closed", cb.State())
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	cb.Execute(func() error { return errors.New("fail") })
	cb.Execute(func() error { return errors.New("fail") })

	time.Sleep(60 * time.Millisecond)
	// Trigger transition to half-open
	cb.Execute(func() error { return nil })
	require.Equal(t, "half-open", cb.State())

	err := cb.Execute(func() error { return errors.New("fail again") })
	require.Error(t, err)
	require.Equal(t, "open", cb.State())
}

func TestCircuitBreaker_ZeroThreshold_Defaults(t *testing.T) {
	cb := NewCircuitBreaker(0, 0, 0)
	require.Equal(t, 5, cb.failureThreshold)
	require.Equal(t, 2, cb.successThreshold)
	require.Equal(t, 30*time.Second, cb.timeout)
}

func TestCircuitBreaker_SuccessInClosedResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	cb.Execute(func() error { return errors.New("fail") })
	cb.Execute(func() error { return errors.New("fail") })
	require.Equal(t, 2, cb.failures)

	cb.Execute(func() error { return nil })
	require.Equal(t, 0, cb.failures)
}

func TestCircuitBreaker_HalfOpenAllowsOnlyOneProbe(t *testing.T) {
	cb := NewCircuitBreaker(1, 1, 20*time.Millisecond)
	cb.Execute(func() error { return errors.New("fail") })
	time.Sleep(30 * time.Millisecond)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- cb.Execute(func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	err := cb.Execute(func() error { return nil })
	require.ErrorIs(t, err, ErrCircuitOpen, "mientras corre la prueba, el resto falla rápido")

	close(release)
	require.NoError(t, <-done)
	require.Equal(t, "closed", cb.State())
}

func TestCircuitBreaker_PanicCountsAsFailure(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, time.Second)

	require.Panics(t, func() {
		cb.Execute(func() error { panic("boom") })
	})

	require.Equal(t, 1, cb.failures)
}

func TestCircuitBreaker_PanicInProbeDoesNotBlockRecovery(t *testing.T) {
	cb := NewCircuitBreaker(1, 1, 20*time.Millisecond)
	cb.Execute(func() error { return errors.New("fail") })
	time.Sleep(30 * time.Millisecond)

	require.Panics(t, func() {
		cb.Execute(func() error { panic("boom") })
	})
	require.Equal(t, "open", cb.State())

	time.Sleep(30 * time.Millisecond)
	require.NoError(t, cb.Execute(func() error { return nil }))
	require.Equal(t, "closed", cb.State())
}

// failingRepo simula una base caída: toda lectura por id devuelve err.
type failingRepo struct {
	domain.DocumentRepository
	err error
}

func (r *failingRepo) FindByDocumentID(context.Context, string) (*domain.Document, error) {
	return nil, r.err
}

func TestCircuitBreakerRepository_PassesResultsThrough(t *testing.T) {
	ctx := context.Background()
	mem := testutil.NewMemoryRepository()
	repo := NewCircuitBreakerRepository(mem, NewCircuitBreaker(2, 2, time.Second))

	doc := testutil.NewDoc("doc-cb-1", "a.pdf", "chk-cb-1", domain.SummaryPending, time.Now())
	require.NoError(t, repo.Insert(ctx, doc))

	got, err := repo.FindByDocumentID(ctx, "doc-cb-1")
	require.NoError(t, err)
	require.Equal(t, "doc-cb-1", got.DocumentID)
}

func TestCircuitBreakerRepository_BusinessErrorsDoNotOpenBreaker(t *testing.T) {
	ctx := context.Background()
	cb := NewCircuitBreaker(2, 2, time.Second)
	repo := NewCircuitBreakerRepository(testutil.NewMemoryRepository(), cb)

	for i := 0; i < 5; i++ {
		_, err := repo.FindByDocumentID(ctx, "no-existe")
		require.ErrorIs(t, err, domain.ErrNotFound)
	}

	require.Equal(t, "closed", cb.State())
}

func TestCircuitBreakerRepository_DuplicateIsBusinessError(t *testing.T) {
	ctx := context.Background()
	cb := NewCircuitBreaker(1, 1, time.Second)
	repo := NewCircuitBreakerRepository(testutil.NewMemoryRepository(), cb)
	doc := testutil.NewDoc("doc-cb-2", "b.pdf", "chk-cb-2", domain.SummaryPending, time.Now())
	require.NoError(t, repo.Insert(ctx, doc))

	err := repo.Insert(ctx, doc)

	require.ErrorIs(t, err, domain.ErrDuplicateDocumentID)
	require.Equal(t, "closed", cb.State())
}

func TestCircuitBreakerRepository_InfraErrorsOpenBreaker(t *testing.T) {
	ctx := context.Background()
	cb := NewCircuitBreaker(2, 2, time.Second)
	repo := NewCircuitBreakerRepository(&failingRepo{err: errors.New("mongo caido")}, cb)

	for i := 0; i < 2; i++ {
		_, err := repo.FindByDocumentID(ctx, "x")
		require.EqualError(t, err, "mongo caido")
	}

	_, err := repo.FindByDocumentID(ctx, "x")
	require.ErrorIs(t, err, ErrCircuitOpen)
	require.Equal(t, "open", cb.State())
}
