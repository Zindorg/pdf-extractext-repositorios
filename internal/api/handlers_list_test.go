package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestHandler_List_OK_PaginationAndFilters(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "a", "resumen 1")
	seedPending(t, svc, "doc-2", "chk-2", "b")
	seedCompleted(t, svc, "doc-3", "chk-3", "c", "resumen 3")
	require.NoError(t, svc.SoftDelete(context.Background(), "doc-3"))

	rr := testutil.DoGet(t, router, "/api/v1/documents")
	require.Equal(t, http.StatusOK, rr.Code)
	var all api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &all))
	require.Len(t, all.Items, 2)
	require.Equal(t, int64(2), all.Total)
	require.Equal(t, 1, all.Page)
	require.Equal(t, 20, all.PageSize)

	rr = testutil.DoGet(t, router, "/api/v1/documents?status=RESOLVED")
	require.Equal(t, http.StatusOK, rr.Code)
	var resolved api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resolved))
	require.Len(t, resolved.Items, 1)
	require.Equal(t, "doc-1", resolved.Items[0].DocumentID)
	require.Equal(t, int64(1), resolved.Total)

	rr = testutil.DoGet(t, router, "/api/v1/documents?page=2&page_size=1")
	require.Equal(t, http.StatusOK, rr.Code)
	var paged api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &paged))
	require.Len(t, paged.Items, 1)
	require.Equal(t, int64(2), paged.Total)
	require.Equal(t, 2, paged.Page)
}

func TestHandler_List_FilterBySummaryStatus(t *testing.T) {
	router, svc := newTestRouter()
	seedCompleted(t, svc, "doc-1", "chk-1", "a", "resumen 1")
	seedPending(t, svc, "doc-2", "chk-2", "b")
	seedWithStatus(t, svc, "doc-3", "chk-3", "c", domain.SummaryNotRequested)
	seedWithStatus(t, svc, "doc-4", "chk-4", "d", domain.SummaryFailed)

	rr := testutil.DoGet(t, router, "/api/v1/documents?status=PENDING")
	require.Equal(t, http.StatusOK, rr.Code)
	var pending api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &pending))
	require.Len(t, pending.Items, 1)
	require.Equal(t, "doc-2", pending.Items[0].DocumentID)
	require.Equal(t, int64(1), pending.Total)

	rr = testutil.DoGet(t, router, "/api/v1/documents?status=NOT_REQUESTED")
	require.Equal(t, http.StatusOK, rr.Code)
	var notreq api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &notreq))
	require.Len(t, notreq.Items, 1)
	require.Equal(t, "doc-3", notreq.Items[0].DocumentID)
	require.Equal(t, int64(1), notreq.Total)

	rr = testutil.DoGet(t, router, "/api/v1/documents?status=FAILED")
	require.Equal(t, http.StatusOK, rr.Code)
	var failed api.ListResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &failed))
	require.Len(t, failed.Items, 0)
	require.Equal(t, int64(0), failed.Total)
}

func TestHandler_List_InvalidStatus_BadRequest(t *testing.T) {
	router, _ := newTestRouter()

	rr := testutil.DoGet(t, router, "/api/v1/documents?status=HECHO")

	require.Equal(t, http.StatusBadRequest, rr.Code)
	requireErrorBody(t, rr, "invalid_request", "invalid status")
}

func TestHandler_List_InvalidCreatedFrom_BadRequest(t *testing.T) {
	router, _ := newTestRouter()

	rr := testutil.DoGet(t, router, "/api/v1/documents?created_from=no-es-fecha")

	require.Equal(t, http.StatusBadRequest, rr.Code)
	requireErrorBody(t, rr, "invalid_request", "invalid created_from")
}
