package redis

import "time"

// RetryDLQ regula los reintentos (XAUTOCLAIM con backoff constante) y decide
// cuándo un fallo agotado va a la dead-letter queue.
type RetryDLQ struct {
	maxRetries int
	backoff    time.Duration
}

func NewRetryDLQ(maxRetries int, backoff time.Duration) *RetryDLQ {
	return &RetryDLQ{maxRetries: maxRetries, backoff: backoff}
}

func (d *RetryDLQ) MaxRetries() int {
	return d.maxRetries
}

func (d *RetryDLQ) Backoff() time.Duration {
	return d.backoff
}

// NeedsDLQ decide si un intento de fallo debe ir a la DLQ.
func (d *RetryDLQ) NeedsDLQ(attempts int) bool {
	return attempts >= d.maxRetries
}
