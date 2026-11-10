package mongodb

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type circuitState int

const (
	stateClosed circuitState = iota
	stateOpen
	stateHalfOpen
)

type CircuitBreaker struct {
	mu               sync.Mutex
	state            circuitState
	failures         int
	successes        int
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	lastFailure      time.Time
}

func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if successThreshold <= 0 {
		successThreshold = 2
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &CircuitBreaker{
		state:            stateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
	}
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	err := fn()
	cb.recordResult(err)
	return err
}

func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		return true
	case stateOpen:
		if time.Since(cb.lastFailure) >= cb.timeout {
			cb.state = stateHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case stateHalfOpen:
		return true
	}
	return false
}

func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailure = time.Now()
		cb.successes = 0

		if cb.state == stateHalfOpen || cb.failures >= cb.failureThreshold {
			cb.state = stateOpen
		}
		return
	}

	cb.failures = 0

	switch cb.state {
	case stateHalfOpen:
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.state = stateClosed
		}
	case stateClosed:
	}
}

func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		return "closed"
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half-open"
	}
	return "unknown"
}
