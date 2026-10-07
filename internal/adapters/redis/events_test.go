package redis

import (
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParseOriginal(t *testing.T) {
	ev, err := parseOriginal(`{
		"event": "original",
		"document_id": "doc-1",
		"checksum": "abc",
		"extracted_text": "texto",
		"metadata": {"filename": "x.pdf", "mime_type": "application/pdf", "size_bytes": 10, "page_count": 2},
		"processing_time_ms": 5
	}`)

	require.NoError(t, err)
	require.Equal(t, "original", ev.Event)
	require.Equal(t, "doc-1", ev.DocumentID)
	require.Equal(t, "abc", ev.Checksum)
	require.Equal(t, "texto", ev.ExtractedText)
	require.Equal(t, domain.Metadata{Filename: "x.pdf", MimeType: "application/pdf", SizeBytes: 10, PageCount: 2}, ev.Metadata)
	require.Equal(t, int64(5), ev.ProcessingTimeMS)
}

func TestParseOriginal_InvalidJSON(t *testing.T) {
	_, err := parseOriginal("{no válido")

	require.Error(t, err)
}

func TestParseSummary(t *testing.T) {
	ev, err := parseSummary(`{"event": "summary", "document_id": "doc-1", "summary": "resumen"}`)

	require.NoError(t, err)
	require.Equal(t, "summary", ev.Event)
	require.Equal(t, "doc-1", ev.DocumentID)
	require.Equal(t, "resumen", ev.Summary)
}

func TestParseSummary_InvalidJSON(t *testing.T) {
	_, err := parseSummary("")

	require.Error(t, err)
}
