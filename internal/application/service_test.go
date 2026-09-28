package application_test

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

// inMemoryRepo es un fake en-memoria del port domain.DocumentRepository:
// implementación real con dedup por document_id/checksum, posta para probar
// el DocumentService sin infraestructura (seam del caso de uso).
type inMemoryRepo struct {
	byID       map[string]domain.Document // document_id → documento
	checksumID map[string]string          // checksum → document_id (activos)
}

func newInMemoryRepo() *inMemoryRepo {
	return &inMemoryRepo{
		byID:       make(map[string]domain.Document),
		checksumID: make(map[string]string),
	}
}

func (r *inMemoryRepo) Insert(_ context.Context, doc domain.Document) error {
	if _, ok := r.byID[doc.DocumentID]; ok {
		return domain.ErrDuplicateDocumentID
	}
	if id, ok := r.checksumID[doc.Checksum]; ok && !r.byID[id].IsDeleted() {
		return domain.ErrDuplicateChecksum
	}
	r.byID[doc.DocumentID] = doc
	r.checksumID[doc.Checksum] = doc.DocumentID
	return nil
}

func (r *inMemoryRepo) UpdateStatus(_ context.Context, _ string, _ domain.Status, _ *string) (*domain.Document, error) {
	return nil, nil // no se usa en el slice 1
}

func (r *inMemoryRepo) FindByDocumentID(_ context.Context, documentID string) (*domain.Document, error) {
	doc, ok := r.byID[documentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &doc, nil
}

func (r *inMemoryRepo) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	id, ok := r.checksumID[checksum]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.FindByDocumentID(ctx, id)
}

func (r *inMemoryRepo) List(_ context.Context, _ domain.ListFilter, _, _ int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}

func (r *inMemoryRepo) SoftDelete(_ context.Context, _ string) error {
	return nil
}

func (r *inMemoryRepo) Restore(_ context.Context, _ string) error {
	return nil
}

func TestCreatePending_StoresPendingDocument(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	payload := domain.Document{
		DocumentID:       "doc-1",
		Checksum:         "abc123",
		ExtractedText:    "contenido extraído",
		ProcessingTimeMS: 42,
		Metadata: domain.Metadata{
			Filename:  "informe.pdf",
			MimeType:  "application/pdf",
			SizeBytes: 2048576,
			PageCount: 12,
		},
	}

	got, err := svc.CreatePending(context.Background(), payload)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.StatusPending, got.Status)
	require.False(t, got.CreatedAt.IsZero())
	require.False(t, got.IsDeleted())

	stored, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, payload.ExtractedText, stored.ExtractedText)
	require.Equal(t, payload.Checksum, stored.Checksum)
	require.Equal(t, payload.Metadata, stored.Metadata)
	require.Equal(t, payload.ProcessingTimeMS, stored.ProcessingTimeMS)
}

func TestCreatePending_DuplicateChecksum(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	first := domain.Document{DocumentID: "doc-1", Checksum: "same-checksum", ExtractedText: "a"}
	_, err := svc.CreatePending(context.Background(), first)
	require.NoError(t, err)

	second := domain.Document{DocumentID: "doc-2", Checksum: "same-checksum", ExtractedText: "b"}
	_, err = svc.CreatePending(context.Background(), second)
	require.ErrorIs(t, err, domain.ErrDuplicateChecksum)
}

func TestCreatePending_DuplicateDocumentID(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	payload := domain.Document{DocumentID: "doc-1", Checksum: "checksum-a"}
	_, err := svc.CreatePending(context.Background(), payload)
	require.NoError(t, err)

	_, err = svc.CreatePending(context.Background(), payload)
	require.ErrorIs(t, err, domain.ErrDuplicateDocumentID)
}