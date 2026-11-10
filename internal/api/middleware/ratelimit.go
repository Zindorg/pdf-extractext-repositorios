package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type rateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rps      rate.Limit
	burst    int
}

func newRateLimiter(rps rate.Limit, burst int) *rateLimiter {
	return &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rps,
		burst:    burst,
	}
}

func (rl *rateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rps, rl.burst)
		rl.limiters[key] = limiter
	}
	return limiter
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, limiter := range rl.limiters {
		if limiter.Tokens() == float64(rl.burst) {
			delete(rl.limiters, key)
		}
	}
}

// RateLimit returns a Gin middleware that limits requests per IP using token bucket.
func RateLimit(rps int, burst int) gin.HandlerFunc {
	if rps <= 0 || burst <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	rl := newRateLimiter(rate.Limit(rps), burst)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    "rate_limited",
				"message": "rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}
