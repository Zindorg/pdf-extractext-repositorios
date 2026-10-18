package middleware

import (
	"github.com/gin-gonic/gin"
)

// RequestID propaga el header X-Request-ID si existe, sin generarlo.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if v := c.GetHeader("X-Request-ID"); v != "" {
			c.Set("request_id", v)
			c.Writer.Header().Set("X-Request-ID", v)
		}
		c.Next()
	}
}
