package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestID_PropagatesHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) {
		v, ok := c.Get("request_id")
		require.True(t, ok)
		require.Equal(t, "req-123", v)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "req-123")
	r.ServeHTTP(w, req)

	require.Equal(t, "req-123", w.Header().Get("X-Request-ID"))
}

func TestRequestID_NoHeader_DoesNotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) {
		_, ok := c.Get("request_id")
		require.False(t, ok)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	require.Empty(t, w.Header().Get("X-Request-ID"))
}

func TestRequestID_EmptyHeader_Ignored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) {
		_, ok := c.Get("request_id")
		require.False(t, ok)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "")
	r.ServeHTTP(w, req)

	require.Empty(t, w.Header().Get("X-Request-ID"))
}
