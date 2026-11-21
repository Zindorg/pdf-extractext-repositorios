package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec)
			}

			attrs := []any{
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"request_id", recoveryRequestID(c),
			}

			// Cliente desconectado (broken pipe / connection reset): no hay a quién responder.
			if isBrokenPipe(rec) {
				slog.Warn("client connection lost", append(attrs, "error", fmt.Sprint(rec))...)
				c.Abort()
				return
			}

			slog.Error("panic recovered", append(attrs,
				"panic", fmt.Sprint(rec),
				"stack", string(debug.Stack()),
			)...)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}()
		c.Next()
	}
}

// isBrokenPipe detecta errores de red por desconexión del cliente.
func isBrokenPipe(rec any) bool {
	ne, ok := rec.(*net.OpError)
	if !ok {
		return false
	}
	var se *os.SyscallError
	if !errors.As(ne, &se) {
		return false
	}
	msg := strings.ToLower(se.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}

// recoveryRequestID obtiene el request_id igual que AccessLog (clave "request_id" del
// contexto de Gin) y, si no está, lo busca en la cabecera de respuesta X-Request-ID.
func recoveryRequestID(c *gin.Context) string {
	if v, ok := c.Get("request_id"); ok {
		if id, ok := v.(string); ok && id != "" {
			return id
		}
	}
	return c.Writer.Header().Get("X-Request-ID")
}
