package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newRequestIDRouter monta un router con el middleware RequestID y el handler
// indicado en la raíz.
func newRequestIDRouter(t *testing.T, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", handler)
	return r
}

func TestRequestID_PropagatesHeader(t *testing.T) {
	r := newRequestIDRouter(t, func(c *gin.Context) {
		v, ok := c.Get("request_id")
		require.True(t, ok)
		require.Equal(t, "req-123", v)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "req-123")
	r.ServeHTTP(w, req)

	require.Equal(t, "req-123", w.Header().Get("X-Request-ID"))
}

// TestRequestID_GeneratesIfMissing: sin cabecera o con cabecera vacía,
// el middleware genera un nuevo request_id y lo propaga en la respuesta.
func TestRequestID_GeneratesIfMissing(t *testing.T) {
	cases := []struct {
		name           string
		setEmptyHeader bool
	}{
		{"sin cabecera", false},
		{"cabecera vacía", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRequestIDRouter(t, func(c *gin.Context) {
				v, ok := c.Get("request_id")
				require.True(t, ok)
				require.NotEmpty(t, v)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.setEmptyHeader {
				req.Header.Set("X-Request-ID", "")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.NotEmpty(t, w.Header().Get("X-Request-ID"))
		})
	}
}
