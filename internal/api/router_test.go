package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	softDeleteErr error
	restoreErr    error
}

func (fakeRepo) Insert(context.Context, domain.Document) error { return nil }
func (fakeRepo) UpdateStatus(context.Context, string, domain.Status, *string, int64) (*domain.Document, error) {
	return nil, nil
}
func (fakeRepo) FindByDocumentID(context.Context, string) (*domain.Document, error) {
	return nil, domain.ErrNotFound
}
func (fakeRepo) FindByChecksum(context.Context, string) (*domain.Document, error) {
	return nil, domain.ErrNotFound
}
func (fakeRepo) List(context.Context, domain.ListFilter, int, int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}
func (r fakeRepo) SoftDelete(context.Context, string) error { return r.softDeleteErr }
func (r fakeRepo) Restore(context.Context, string) error    { return r.restoreErr }

type healthy struct{}

func (healthy) Ping() error { return nil }

type unhealthy struct{}

func (unhealthy) Ping() error { return errors.New("down") }

func TestHealthOK(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
}

func TestHealthUnavailable(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{})
	handler := api.NewDocumentHandler(service, unhealthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestHandler_SoftDelete_Success(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/documents/doc-1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandler_SoftDelete_NotFound(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{softDeleteErr: domain.ErrNotFound})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/documents/doc-1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_Restore_Success(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/restore", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandler_Restore_NotFound(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{restoreErr: domain.ErrNotFound})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/restore", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_Restore_ConflictChecksum(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{restoreErr: domain.ErrRestoreConflict})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/restore", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
}
