package application_test

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestCreatePending_StoresPendingDocument(t *testing.T) {
	svc, repo := newService()

	payload := domain.Document{
		DocumentID:       "doc-1",
		Checksum:         "abc123",
		SummaryStatus:    domain.SummaryPending,
		ExtractedText:    "contenido extraído",
		ExtractionTimeMS: 40,
		SummaryTimeMS:    2,
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
	require.Equal(t, domain.SummaryPending, got.SummaryStatus)
	require.False(t, got.CreatedAt.IsZero())
	require.False(t, got.IsDeleted())

	stored, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, payload.ExtractedText, stored.ExtractedText)
	require.Equal(t, payload.Checksum, stored.Checksum)
	require.Equal(t, payload.Metadata, stored.Metadata)
	require.Equal(t, payload.ExtractionTimeMS, stored.ExtractionTimeMS)
	require.Equal(t, payload.SummaryTimeMS, stored.SummaryTimeMS)
}

func TestCreatePending_DuplicateChecksum(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "same-checksum", "a")

	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID: "doc-2", Checksum: "same-checksum", ExtractedText: "b",
	})
	require.ErrorIs(t, err, domain.ErrDuplicateChecksum)
}

func TestCreatePending_DuplicateDocumentID(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "checksum-a", "")

	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "checksum-a",
	})
	require.ErrorIs(t, err, domain.ErrDuplicateDocumentID)
}

func TestCompleteWithSummary_CompletesPendingDocument(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "abc123", "contenido original")

	got, err := svc.CompleteWithSummary(context.Background(), "doc-1", "resumen del doc", 120)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.SummaryResolved, got.SummaryStatus)
	require.NotNil(t, got.Summary)
	require.Equal(t, "resumen del doc", *got.Summary)
	require.Equal(t, int64(120), got.SummaryTimeMS)
	require.True(t, got.UpdatedAt.After(got.CreatedAt))
}

func TestCompleteWithSummary_OrphanSummary_ReturnsErrNotFound(t *testing.T) {
	svc, _ := newService()

	_, err := svc.CompleteWithSummary(context.Background(), "no-existe", "resumen huérfano", 120)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCompleteWithSummary_AlreadyCompleted_IsBenign(t *testing.T) {
	svc, repo := newService()

	seedPending(t, svc, "doc-1", "abc123", "")

	_, err := svc.CompleteWithSummary(context.Background(), "doc-1", "primera vez", 120)
	require.NoError(t, err)

	// Duplicado legítimo (entrega at-least-once): sin error → consumer hace XACK + descarte.
	_, err = svc.CompleteWithSummary(context.Background(), "doc-1", "otra vez", 200)
	require.NoError(t, err)

	stored, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.SummaryResolved, stored.SummaryStatus)
	require.Equal(t, "primera vez", *stored.Summary)
	require.Equal(t, int64(120), stored.SummaryTimeMS)
}
