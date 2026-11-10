package api_test

import (
	"net/http"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestHealthOK(t *testing.T) {
	router := newRouter(application.NewDocumentService(fakeRepo{}), healthy{})

	rr := testutil.DoGet(t, router, "/api/v1/health")

	require.Equal(t, http.StatusOK, rr.Code)
	require.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
}

func TestHealthUnavailable(t *testing.T) {
	router := newRouter(application.NewDocumentService(fakeRepo{}), unhealthy{})

	rr := testutil.DoGet(t, router, "/api/v1/health")

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestHandler_SoftDelete_Success(t *testing.T) {
	router := newRouter(application.NewDocumentService(fakeRepo{}), healthy{})

	rr := testutil.DoDelete(t, router, "/api/v1/documents/doc-1")

	require.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandler_SoftDelete_NotFound(t *testing.T) {
	router := newRouter(application.NewDocumentService(fakeRepo{softDeleteErr: domain.ErrNotFound}), healthy{})

	rr := testutil.DoDelete(t, router, "/api/v1/documents/doc-1")

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_Restore_Success(t *testing.T) {
	router := newRouter(application.NewDocumentService(fakeRepo{}), healthy{})

	rr := testutil.DoPost(t, router, "/api/v1/documents/doc-1/restore")

	require.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandler_Restore_NotFound(t *testing.T) {
	router := newRouter(application.NewDocumentService(fakeRepo{restoreErr: domain.ErrNotFound}), healthy{})

	rr := testutil.DoPost(t, router, "/api/v1/documents/doc-1/restore")

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_Restore_ConflictChecksum(t *testing.T) {
	router := newRouter(application.NewDocumentService(fakeRepo{restoreErr: domain.ErrRestoreConflict}), healthy{})

	rr := testutil.DoPost(t, router, "/api/v1/documents/doc-1/restore")

	require.Equal(t, http.StatusConflict, rr.Code)
}
