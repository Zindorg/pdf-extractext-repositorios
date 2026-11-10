package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestToResponse(t *testing.T) {
	doc := testutil.CompleteDoc()

	got := toResponse(doc, true)

	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, "abc", got.Checksum)
	require.Equal(t, "RESOLVED", got.SummaryStatus)
	require.Equal(t, "texto", got.ExtractedText)
	require.Equal(t, doc.Summary, got.Summary)
	require.Equal(t, doc.Metadata, got.Metadata)
	// processing_time_ms = extraction + summary (la API expone la suma total)
	require.Equal(t, int64(5), got.ProcessingTimeMS)
	require.Equal(t, doc.CreatedAt, got.CreatedAt)
	require.Equal(t, doc.UpdatedAt, got.UpdatedAt)
}

func TestToResponse_NilSummary(t *testing.T) {
	got := toResponse(domain.Document{}, true)

	require.Nil(t, got.Summary)
	require.Equal(t, "", got.SummaryStatus)
}

func TestToResponse_DeliveryFilenameAndMimeType(t *testing.T) {
	doc := domain.Document{
		DocumentID:    "doc-1",
		ExtractedText: "texto",
		Metadata: domain.Metadata{
			Filename: "informe.pdf",
			MimeType: "application/pdf",
		},
	}

	raw, err := json.Marshal(toResponse(doc, true))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	require.Equal(t, "AVAILABLE", got["delivery_status"])
	require.Equal(t, "informe.pdf", got["filename"])
	require.Equal(t, "application/pdf", got["mime_type"])
}

func TestToResponse_TimingsAndDeletedAt(t *testing.T) {
	summary := "resumen"
	deletedAt := time.Now().UTC()
	doc := domain.Document{
		DocumentID:       "doc-1",
		Summary:          &summary,
		ExtractionTimeMS: 342,
		SummaryTimeMS:    120,
		DeletedAt:        &deletedAt,
	}

	got := marshalResponse(t, doc)

	require.Equal(t, float64(342), got["extraction_time_ms"])
	require.Equal(t, float64(120), got["summary_time_ms"])
	require.NotNil(t, got["deleted_at"])
}

func TestToResponse_NoSummaryOmitsNullableFields(t *testing.T) {
	got := marshalResponse(t, domain.Document{DocumentID: "doc-1", ExtractionTimeMS: 342})

	require.Equal(t, float64(342), got["extraction_time_ms"])
	// summary_time_ms y deleted_at se omiten (omitempty) cuando son nil
	_, hasSummaryTime := got["summary_time_ms"]
	require.False(t, hasSummaryTime)
	_, hasDeletedAt := got["deleted_at"]
	require.False(t, hasDeletedAt)
}

func marshalResponse(t *testing.T, doc domain.Document) map[string]any {
	t.Helper()
	raw, err := json.Marshal(toResponse(doc, true))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	return got
}
