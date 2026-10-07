package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/api"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/stretchr/testify/require"
)

func TestBusinessEndpointsNotImplemented(t *testing.T) {
	service := application.NewDocumentService(fakeRepo{})
	handler := api.NewDocumentHandler(service, healthy{})
	router := api.NewRouter(handler)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{name: "get by id", method: http.MethodGet, path: "/api/v1/documents/abc"},
		{name: "get by checksum", method: http.MethodGet, path: "/api/v1/documents/checksum/chk"},
		{name: "list", method: http.MethodGet, path: "/api/v1/documents"},
		{name: "download original", method: http.MethodGet, path: "/api/v1/documents/abc/download/original"},
		{name: "download summary", method: http.MethodGet, path: "/api/v1/documents/abc/download/summary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusNotImplemented, rr.Code)
			require.JSONEq(t, `{"code":"NOT_IMPLEMENTED","message":"fase de lógica pendiente"}`, rr.Body.String())
		})
	}
}
