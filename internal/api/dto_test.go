package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestToResponse(t *testing.T) {
	summary := "resumen"
	created := time.Now().UTC()
	doc := domain.Document{
		DocumentID:       "doc-1",
		Checksum:         "abc",
		SummaryStatus:    domain.SummaryResolved,
		ExtractedText:    "texto",
		Summary:          &summary,
		Metadata:         domain.Metadata{Filename: "x.pdf", MimeType: "application/pdf", SizeBytes: 10, PageCount: 2},
		ExtractionTimeMS: 3,
		SummaryTimeMS:    2,
		CreatedAt:        created,
		UpdatedAt:        created,
	}

	got := toResponse(doc)

	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, "abc", got.Checksum)
	require.Equal(t, "RESOLVED", got.SummaryStatus)
	require.Equal(t, "texto", got.ExtractedText)
	require.Equal(t, &summary, got.Summary)
	require.Equal(t, doc.Metadata, got.Metadata)
	// processing_time_ms = extraction + summary (la API expone la suma total)
	require.Equal(t, int64(5), got.ProcessingTimeMS)
	require.Equal(t, created, got.CreatedAt)
	require.Equal(t, created, got.UpdatedAt)
}

func TestToResponse_NilSummary(t *testing.T) {
	got := toResponse(domain.Document{})

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

	raw, err := json.Marshal(toResponse(doc))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	require.Equal(t, "AVAILABLE", got["delivery_status"])
	require.Equal(t, "informe.pdf", got["filename"])
	require.Equal(t, "application/pdf", got["mime_type"])
}
