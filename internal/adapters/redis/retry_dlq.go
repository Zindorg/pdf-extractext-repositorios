package redis

import "time"

// RetryDLQ maneja reintentos (XAUTOCLAIM con backoff) y el envío a la
// dead-letter queue. En esta fase solo declara la superficie; la lógica de
// Redis y el cálculo de retries se completa en la fase de lógica.
type RetryDLQ struct {
	maxRetries    int
	backoff       time.Duration
	retryCountKey string
}

const (
	defaultRetryCountField = "retry_count"
	dlqCauseField          = "dlq_cause"
)

func NewRetryDLQ(maxRetries int, backoff time.Duration) *RetryDLQ {
	return &RetryDLQ{
		maxRetries:    maxRetries,
		backoff:       backoff,
		retryCountKey: defaultRetryCountField,
	}
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
