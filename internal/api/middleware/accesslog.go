package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func AccessLog(quietPaths ...string) gin.HandlerFunc {
	quiet := make(map[string]struct{}, len(quietPaths))
	for _, p := range quietPaths {
		quiet[p] = struct{}{}
	}

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

		if id, ok := requestID.(string); ok && id != "" {
			attrs = append(attrs, slog.String("request_id", id))
		}

		// Nivel según el status; las rutas de sondeo exitosas bajan a debug.
		switch {
		case status >= 500:
			slog.Error("HTTP request completed", attrs...)
		case status >= 400:
			slog.Warn("HTTP request completed", attrs...)
		default:
			if _, isQuiet := quiet[path]; isQuiet {
				slog.Debug("HTTP request completed", attrs...)
			} else {
				slog.Info("HTTP request completed", attrs...)
			}
		}
	}
}
