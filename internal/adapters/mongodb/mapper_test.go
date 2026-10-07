package mongodb

import (
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestPersistedRoundTrip(t *testing.T) {
	summary := "resumen"
	deletedAt := time.Now().UTC()
	created := time.Now().UTC()
	doc := domain.Document{
		DocumentID:       "doc-1",
		Checksum:         "abc",
		Status:           domain.StatusCompleted,
		ExtractedText:    "texto",
		Summary:          &summary,
		Metadata:         domain.Metadata{Filename: "x.pdf", MimeType: "application/pdf", SizeBytes: 10, PageCount: 2},
		ExtractionTimeMS: 3,
		SummaryTimeMS:    2,
		CreatedAt:        created,
		UpdatedAt:        created,
		DeletedAt:        &deletedAt,
	}

	got := toDomain(toPersisted(doc))

	require.Equal(t, doc.DocumentID, got.DocumentID)
	require.Equal(t, doc.Checksum, got.Checksum)
	require.Equal(t, doc.Status, got.Status)
	require.Equal(t, doc.ExtractedText, got.ExtractedText)
	require.Equal(t, doc.Summary, got.Summary)
	require.Equal(t, doc.Metadata, got.Metadata)
	require.Equal(t, doc.ExtractionTimeMS, got.ExtractionTimeMS)
	require.Equal(t, doc.SummaryTimeMS, got.SummaryTimeMS)
	require.Equal(t, doc.CreatedAt, got.CreatedAt)
	require.Equal(t, doc.UpdatedAt, got.UpdatedAt)
	require.Equal(t, doc.DeletedAt, got.DeletedAt)
}

func TestPersistedRoundTrip_NoSummaryNoDeletedAt(t *testing.T) {
	got := toDomain(toPersisted(domain.Document{DocumentID: "doc-2"}))

	require.Equal(t, "doc-2", got.DocumentID)
	require.Nil(t, got.Summary)
	require.Nil(t, got.DeletedAt)
	require.Equal(t, domain.Status(""), got.Status)
}
