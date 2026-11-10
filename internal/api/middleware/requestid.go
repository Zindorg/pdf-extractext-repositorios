package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequestID propaga el header X-Request-ID si existe, o genera uno nuevo.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		} else {
			requestID = sanitizeRequestID(requestID)
		}
		c.Set("request_id", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
	}
}

// sanitizeRequestID elimina caracteres de control que podrían forjar logs
// (newline, carriage return, tab).
func sanitizeRequestID(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, s)
}

// generateRequestID genera un ID aleatorio de 16 bytes (32 chars hex).
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
