package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

// newTestRouter construye la API sobre el MemoryRepository compartido (testutil)
// con el DocumentService real, de modo que los tests ejercen handler + service.
func newTestRouter() (*gin.Engine, *application.DocumentService) {
	repo := testutil.NewMemoryRepository()
	service := application.NewDocumentService(repo)
	handler := api.NewDocumentHandler(service, healthy{})
	return api.NewRouter(handler), service
}

// seedPending inserta un documento PENDING vía el servicio.
func seedPending(t *testing.T, svc *application.DocumentService, id, checksum, text string) {
	t.Helper()
	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID: id, Checksum: checksum, ExtractedText: text,
		SummaryStatus:    domain.SummaryPending,
		Metadata:         domain.Metadata{Filename: "mi archivo.pdf", MimeType: "application/pdf"},
		ExtractionTimeMS: 342,
	})
	require.NoError(t, err)
}

// seedCompleted inserta un documento con el resumen resuelto vía el servicio.
func seedCompleted(t *testing.T, svc *application.DocumentService, id, checksum, text, summary string) {
	t.Helper()
	seedPending(t, svc, id, checksum, text)
	_, err := svc.CompleteWithSummary(context.Background(), id, summary, 120)
	require.NoError(t, err)
}

func doGet(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestHandler_GetByDocumentID_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto extraido", "resumen")

	rr := doGet(t, router, "/api/v1/documents/doc-1")
	require.Equal(t, http.StatusOK, rr.Code)

	var got api.DocumentResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, "chk-1", got.Checksum)
	require.Equal(t, string(domain.SummaryResolved), got.SummaryStatus)
	require.Equal(t, "texto extraido", got.ExtractedText)
	require.Equal(t, "resumen", *got.Summary)
	require.Equal(t, int64(462), got.ProcessingTimeMS)
	require.Equal(t, "mi archivo.pdf", got.Metadata.Filename)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
	require.Equal(t, "AVAILABLE", raw["delivery_status"])
	require.Equal(t, "mi archivo.pdf", raw["filename"])
	require.Equal(t, "application/pdf", raw["mime_type"])
	require.Equal(t, float64(342), raw["extraction_time_ms"])
	require.Equal(t, float64(120), raw["summary_time_ms"])
}

func TestHandler_GetByDocumentID_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := doGet(t, router, "/api/v1/documents/no-existe")
	require.Equal(t, http.StatusNotFound, rr.Code)
	require.JSONEq(t, `{"code":"NOT_FOUND","message":"document not found"}`, rr.Body.String())
}

func TestHandler_GetByDocumentID_SoftDeleted_IsNotFound(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto", "resumen")
	require.NoError(t, svc.SoftDelete(context.Background(), "doc-1"))

	rr := doGet(t, router, "/api/v1/documents/doc-1")
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_GetByChecksum_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedPending(t, svc, "doc-1", "chk-1", "texto")

	rr := doGet(t, router, "/api/v1/documents/checksum/chk-1")
	require.Equal(t, http.StatusOK, rr.Code)

	var got api.DocumentResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "doc-1", got.DocumentID)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
	require.Equal(t, float64(342), raw["extraction_time_ms"])
	summaryTime, ok := raw["summary_time_ms"]
	require.True(t, ok, "summary_time_ms debe estar presente y ser null")
	require.Nil(t, summaryTime)
}

func TestHandler_GetByChecksum_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := doGet(t, router, "/api/v1/documents/checksum/no-existe")
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_List_OK_PaginationAndFilters(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "a", "resumen 1")
	seedPending(t, svc, "doc-2", "chk-2", "b")
	seedCompleted(t, svc, "doc-3", "chk-3", "c", "resumen 3")
	require.NoError(t, svc.SoftDelete(context.Background(), "doc-3"))

	rr := doGet(t, router, "/api/v1/documents")
	require.Equal(t, http.StatusOK, rr.Code)
	var all api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &all))
	require.Len(t, all.Items, 2)
	require.Equal(t, int64(2), all.Total)
	require.Equal(t, 1, all.Page)
	require.Equal(t, 20, all.PageSize)

	rr = doGet(t, router, "/api/v1/documents?status=RESOLVED")
	require.Equal(t, http.StatusOK, rr.Code)
	var resolved api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resolved))
	require.Len(t, resolved.Items, 1)
	require.Equal(t, "doc-1", resolved.Items[0].DocumentID)
	require.Equal(t, int64(1), resolved.Total)

	rr = doGet(t, router, "/api/v1/documents?page=2&page_size=1")
	require.Equal(t, http.StatusOK, rr.Code)
	var paged api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &paged))
	require.Len(t, paged.Items, 1)
	require.Equal(t, int64(2), paged.Total)
	require.Equal(t, 2, paged.Page)
}

func TestHandler_List_InvalidStatus_BadRequest(t *testing.T) {
	router, _ := newTestRouter()

	rr := doGet(t, router, "/api/v1/documents?status=HECHO")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.JSONEq(t, `{"code":"BAD_REQUEST","message":"invalid status"}`, rr.Body.String())
}

func TestHandler_List_InvalidCreatedFrom_BadRequest(t *testing.T) {
	router, _ := newTestRouter()

	rr := doGet(t, router, "/api/v1/documents?created_from=no-es-fecha")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.JSONEq(t, `{"code":"BAD_REQUEST","message":"invalid created_from"}`, rr.Body.String())
}

func TestHandler_DownloadOriginal_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto extraido", "resumen")

	rr := doGet(t, router, "/api/v1/documents/doc-1/download/original")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "texto extraido", rr.Body.String())
	require.Contains(t, rr.Header().Get("Content-Type"), "text/plain")
	expected := "attachment; filename*=UTF-8''" + url.PathEscape("mi archivo.pdf")
	require.Equal(t, expected, rr.Header().Get("Content-Disposition"))
}

func TestHandler_DownloadOriginal_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := doGet(t, router, "/api/v1/documents/no-existe/download/original")
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_DownloadSummary_OK(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "texto", "resumen final")

	rr := doGet(t, router, "/api/v1/documents/doc-1/download/summary")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "resumen final", rr.Body.String())
	require.Contains(t, rr.Header().Get("Content-Type"), "text/plain")
	expected := "attachment; filename*=UTF-8''summary-doc-1.txt"
	require.Equal(t, expected, rr.Header().Get("Content-Disposition"))
}

func TestHandler_DownloadSummary_Pending_Conflict(t *testing.T) {
	router, svc := newTestRouter()
	seedPending(t, svc, "doc-1", "chk-1", "texto")

	rr := doGet(t, router, "/api/v1/documents/doc-1/download/summary")
	require.Equal(t, http.StatusConflict, rr.Code)
	require.JSONEq(t, `{"code":"summary_pending","message":"summary not ready"}`, rr.Body.String())
}

func TestHandler_DownloadSummary_NotFound(t *testing.T) {
	router, _ := newTestRouter()

	rr := doGet(t, router, "/api/v1/documents/no-existe/download/summary")
	require.Equal(t, http.StatusNotFound, rr.Code)
}
