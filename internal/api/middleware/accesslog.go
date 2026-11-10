package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog devuelve un middleware que registra cada petición en JSON
// con slog, incluyendo request_id, método, path, status, latencia, etc.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		requestID, _ := c.Get("request_id")

		attrs := []any{
			slog.String("method", method),
			slog.String("path", path),
			slog.String("query", query),
			slog.Int("status", status),
			slog.String("client_ip", clientIP),
			slog.Duration("latency", latency),
		}

		if requestID != nil {
			attrs = append(attrs, slog.String("request_id", requestID.(string)))
		}

		// Log level based on status code
		if status >= 500 {
			slog.Error("HTTP request completed", attrs...)
		} else if status >= 400 {
			slog.Warn("HTTP request completed", attrs...)
		} else {
			slog.Info("HTTP request completed", attrs...)
		}
	}
}
