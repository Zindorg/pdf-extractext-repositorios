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

// TestRequestID_NoHeader_DoesNotSet: sin cabecera, o con cabecera vacía, la
// petición no recibe request_id ni lo propaga en la respuesta.
func TestRequestID_NoHeader_DoesNotSet(t *testing.T) {
	cases := map[string]bool{
		"sin cabecera":   false,
		"cabecera vacía": true,
	}

	for name, withEmptyHeader := range cases {
		t.Run(name, func(t *testing.T) {
			requireNoRequestID(t, withEmptyHeader)
		})
	}
}

// requireNoRequestID comprueba que la petición no recibe request_id ni la
// cabecera de respuesta.
func requireNoRequestID(t *testing.T, withEmptyHeader bool) {
	t.Helper()
	r := newRequestIDRouter(t, func(c *gin.Context) {
		_, ok := c.Get("request_id")
		require.False(t, ok)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if withEmptyHeader {
		req.Header.Set("X-Request-ID", "")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Empty(t, w.Header().Get("X-Request-ID"))
}
