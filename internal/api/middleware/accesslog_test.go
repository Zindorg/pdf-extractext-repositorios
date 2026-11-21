package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/api/middleware"
	"github.com/stretchr/testify/require"
)

// captureQuietLogs redirige slog a un buffer JSON con el nivel indicado y restaura el logger al terminar.
func captureQuietLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func newQuietRouter(healthStatus int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.AccessLog("/api/v1/health"))
	r.GET("/api/v1/health", func(c *gin.Context) { c.Status(healthStatus) })
	r.GET("/api/v1/documents", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestAccessLogQuietPathHiddenAtInfo(t *testing.T) {
	buf := captureQuietLogs(t, slog.LevelInfo)
	r := newQuietRouter(http.StatusOK)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	require.Empty(t, buf.String(), "el healthcheck exitoso no debe registrarse en nivel info")
}

func TestAccessLogQuietPathVisibleAtDebug(t *testing.T) {
	buf := captureQuietLogs(t, slog.LevelDebug)
	r := newQuietRouter(http.StatusOK)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	require.Contains(t, buf.String(), `"level":"DEBUG"`)
	require.Contains(t, buf.String(), `"path":"/api/v1/health"`)
}

func TestAccessLogQuietPathStillLogsFailures(t *testing.T) {
	buf := captureQuietLogs(t, slog.LevelInfo)
	r := newQuietRouter(http.StatusServiceUnavailable)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	require.Contains(t, buf.String(), `"level":"ERROR"`)
	require.Contains(t, buf.String(), `"status":503`)
}

func TestAccessLogRegularPathLogsAtInfo(t *testing.T) {
	buf := captureQuietLogs(t, slog.LevelInfo)
	r := newQuietRouter(http.StatusOK)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/documents", nil))

	require.Contains(t, buf.String(), `"level":"INFO"`)
	require.Contains(t, buf.String(), `"path":"/api/v1/documents"`)
}
