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

type fakeRepo struct{}

func (fakeRepo) Insert(context.Context, domain.Document) error { return nil }
func (fakeRepo) UpdateStatus(context.Context, string, domain.Status, *string) (*domain.Document, error) {
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
func (fakeRepo) SoftDelete(context.Context, string) error { return nil }
func (fakeRepo) Restore(context.Context, string) error    { return nil }

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

func TestGetByDocumentIDNotImplemented(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/abc", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotImplemented, rr.Code)
}
