package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// doRequest ejecuta method/path contra handler y devuelve la respuesta
// grabada. Acepta cualquier http.Handler (un *gin.Engine lo es), así que
// testutil no depende del framework de routing.
func doRequest(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// DoGet ejecuta una petición GET contra handler.
func DoGet(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, handler, http.MethodGet, path)
}

// DoDelete ejecuta una petición DELETE contra handler.
func DoDelete(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, handler, http.MethodDelete, path)
}

// DoPost ejecuta una petición POST sin cuerpo contra handler.
func DoPost(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, handler, http.MethodPost, path)
}
