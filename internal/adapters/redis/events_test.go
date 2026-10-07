package redis

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOriginal(t *testing.T) {
	ev, err := parseOriginal(`{
		"event_type": "original",
		"event": "original",
		"schema_version": 1,
		"document_id": "doc-1",
		"checksum": "abc",
		"extracted_text": "texto",
		"extraction_time_ms": 342,
		"mime_type": "application/pdf",
		"summary_requested": true,
		"summary_status": "PENDING",
		"metadata": {"filename": "x.pdf", "size_bytes": 10, "page_count": 2}
	}`)

	require.NoError(t, err)
	require.Equal(t, "doc-1", ev.DocumentID)
	require.Equal(t, "abc", ev.Checksum)
	require.Equal(t, "texto", ev.ExtractedText)
	require.Equal(t, int64(342), ev.ExtractionTimeMS)
	require.Equal(t, "application/pdf", ev.MimeType)
	require.True(t, ev.SummaryRequested)
	require.Equal(t, "PENDING", ev.SummaryStatus)
	require.Equal(t, "x.pdf", ev.Metadata.Filename)
	require.Equal(t, int64(10), ev.Metadata.SizeBytes)
	require.Equal(t, 2, ev.Metadata.PageCount)
}

func TestParseOriginal_EventFallback(t *testing.T) {
	// Discriminador tolerante: sin event_type, se despacha por event.
	ev, err := parseOriginal(`{
		"event": "original",
		"schema_version": 1,
		"document_id": "doc-1",
		"checksum": "abc",
		"extracted_text": "texto",
		"mime_type": "application/pdf",
		"metadata": {"filename": "x.pdf"}
	}`)

	require.NoError(t, err)
	require.Equal(t, "doc-1", ev.DocumentID)
}

func TestParseOriginal_UnsupportedSchema(t *testing.T) {
	_, err := parseOriginal(`{
		"event": "original",
		"schema_version": 2,
		"document_id": "doc-1",
		"checksum": "abc",
		"extracted_text": "texto",
		"metadata": {"filename": "x.pdf"}
	}`)

	require.Error(t, err)
}

func TestParseOriginal_MissingRequiredField(t *testing.T) {
	_, err := parseOriginal(`{
		"event": "original",
		"schema_version": 1,
		"checksum": "abc",
		"extracted_text": "texto",
		"metadata": {"filename": "x.pdf"}
	}`)

	require.Error(t, err)
}

func TestParseOriginal_DiscriminatorMismatch(t *testing.T) {
	_, err := parseOriginal(`{
		"event_type": "original",
		"event": "summary_resolved",
		"schema_version": 1,
		"document_id": "d",
		"checksum": "c",
		"extracted_text": "t",
		"metadata": {"filename": "f"}
	}`)

	require.Error(t, err)
}

func TestParseOriginal_InvalidJSON(t *testing.T) {
	_, err := parseOriginal("{no válido")

	require.Error(t, err)
}

func TestParseSummaryResolved(t *testing.T) {
	ev, err := parseSummaryResolved(`{
		"event_type": "summary_resolved",
		"event": "summary_resolved",
		"schema_version": 1,
		"document_id": "doc-1",
		"summary": "resumen",
		"summary_time_ms": 120,
		"summary_status": "RESOLVED"
	}`)

	require.NoError(t, err)
	require.Equal(t, "doc-1", ev.DocumentID)
	require.Equal(t, "resumen", ev.Summary)
	require.Equal(t, int64(120), ev.SummaryTimeMS)
	require.Equal(t, "RESOLVED", ev.SummaryStatus)
}

func TestParseSummaryResolved_UnsupportedSchema(t *testing.T) {
	_, err := parseSummaryResolved(`{
		"event_type": "summary_resolved",
		"schema_version": 0,
		"document_id": "doc-1",
		"summary": "resumen"
	}`)

	require.Error(t, err)
}

func TestParseSummaryResolved_MissingSummary(t *testing.T) {
	_, err := parseSummaryResolved(`{
		"event": "summary_resolved",
		"schema_version": 1,
		"document_id": "doc-1"
	}`)

	require.Error(t, err)
}

func TestParseSummaryResolved_InvalidJSON(t *testing.T) {
	_, err := parseSummaryResolved("")

	require.Error(t, err)
}

func TestDiscriminator_PrefersEventType(t *testing.T) {
	kind, err := discriminator(`{"event_type": "original", "event": "original"}`)

	require.NoError(t, err)
	require.Equal(t, eventOriginal, kind)
}

func TestDiscriminator_FallsBackToEvent(t *testing.T) {
	kind, err := discriminator(`{"event": "summary_resolved"}`)

	require.NoError(t, err)
	require.Equal(t, eventSummaryResolved, kind)
}

func TestDiscriminator_Mismatch(t *testing.T) {
	_, err := discriminator(`{"event_type": "original", "event": "summary_resolved"}`)

	require.Error(t, err)
}

func TestDiscriminator_Missing(t *testing.T) {
	_, err := discriminator(`{}`)

	require.Error(t, err)
}

func TestDiscriminator_UnknownKindIsRouted(t *testing.T) {
	// discriminator no valida el valor (responsabilidad de quien rootea):
	// un event_type desconocido sale igual y se rechaza en el proceso.
	kind, err := discriminator(`{"event": "summary"}`)

	require.NoError(t, err)
	require.Equal(t, "summary", kind)
}
