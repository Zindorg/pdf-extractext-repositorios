package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestHandler_GetByDocumentID_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto extraido", "resumen")

	got, raw := documentResponse(t, router, "/api/v1/documents/doc-1")

	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, "chk-1", got.Checksum)
	require.Equal(t, string(domain.SummaryResolved), got.SummaryStatus)
	require.Equal(t, "texto extraido", got.ExtractedText)
	require.Equal(t, "resumen", *got.Summary)
	require.Equal(t, int64(462), got.ProcessingTimeMS)
	require.Equal(t, "mi archivo.pdf", got.Metadata.Filename)

	require.Equal(t, "AVAILABLE", raw["delivery_status"])
	require.Equal(t, "mi archivo.pdf", raw["filename"])
	require.Equal(t, "application/pdf", raw["mime_type"])
	require.Equal(t, float64(342), raw["extraction_time_ms"])
	require.Equal(t, float64(120), raw["summary_time_ms"])
}

func TestHandler_GetByDocumentID_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := testutil.DoGet(t, router, "/api/v1/documents/no-existe")

	require.Equal(t, http.StatusNotFound, rr.Code)
	requireErrorBody(t, rr, "document_not_found", "document not found")
}

func TestHandler_GetByDocumentID_SoftDeleted_IsNotFound(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto", "resumen")
	require.NoError(t, svc.SoftDelete(context.Background(), "doc-1"))

	rr := testutil.DoGet(t, router, "/api/v1/documents/doc-1")

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_GetByChecksum_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedPending(t, svc, "doc-1", "chk-1", "texto")

	rr := testutil.DoGet(t, router, "/api/v1/documents/checksum/chk-1")
	require.Equal(t, http.StatusOK, rr.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))

	require.Equal(t, "doc-1", raw["document_id"])
	require.Equal(t, "PENDING", raw["summary_status"])
	require.Nil(t, raw["summary"])
	// extraction_time_ms no se incluye en la respuesta de checksum
	_, hasExtractionTime := raw["extraction_time_ms"]
	require.False(t, hasExtractionTime)
}

func TestHandler_GetByChecksum_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := testutil.DoGet(t, router, "/api/v1/documents/checksum/no-existe")

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_DownloadOriginal_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto extraido", "resumen")

	rr := testutil.DoGet(t, router, "/api/v1/documents/doc-1/download/original")

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "texto extraido", rr.Body.String())
	require.Contains(t, rr.Header().Get("Content-Type"), "text/plain")
	expected := "attachment; filename*=UTF-8''" + url.PathEscape("mi archivo.pdf")
	require.Equal(t, expected, rr.Header().Get("Content-Disposition"))
}

func TestHandler_DownloadOriginal_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := testutil.DoGet(t, router, "/api/v1/documents/no-existe/download/original")

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_DownloadSummary_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto", "resumen final")

	rr := testutil.DoGet(t, router, "/api/v1/documents/doc-1/download/summary")

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "resumen final", rr.Body.String())
	require.Contains(t, rr.Header().Get("Content-Type"), "text/plain")
	expected := "attachment; filename*=UTF-8''summary-doc-1.txt"
	require.Equal(t, expected, rr.Header().Get("Content-Disposition"))
}

func TestHandler_DownloadSummary_Pending_Conflict(t *testing.T) {
	router, svc := newTestRouter()
	seedPending(t, svc, "doc-1", "chk-1", "texto")

	rr := testutil.DoGet(t, router, "/api/v1/documents/doc-1/download/summary")

	require.Equal(t, http.StatusConflict, rr.Code)
	requireErrorBody(t, rr, "summary_pending", "summary not ready")
}

func TestHandler_DownloadSummary_NotRequested_NotFound(t *testing.T) {
	router, svc := newTestRouter()
	seedWithStatus(t, svc, "doc-1", "chk-1", "texto", domain.SummaryNotRequested)

	rr := testutil.DoGet(t, router, "/api/v1/documents/doc-1/download/summary")

	require.Equal(t, http.StatusNotFound, rr.Code)
	requireErrorBody(t, rr, "summary_not_available", "summary not available")
}

func TestHandler_DownloadSummary_Failed_NotFound(t *testing.T) {
	router, svc := newTestRouter()
	seedWithStatus(t, svc, "doc-1", "chk-1", "texto", domain.SummaryFailed)

	rr := testutil.DoGet(t, router, "/api/v1/documents/doc-1/download/summary")

	require.Equal(t, http.StatusNotFound, rr.Code)
	requireErrorBody(t, rr, "summary_not_available", "summary not available")
}

func TestHandler_DownloadSummary_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := testutil.DoGet(t, router, "/api/v1/documents/no-existe/download/summary")

	require.Equal(t, http.StatusNotFound, rr.Code)
}
