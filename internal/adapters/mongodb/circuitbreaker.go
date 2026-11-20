package mongodb

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrCircuitOpen = errors.New("circuit breaker open")
	errPanicInCall = errors.New("panic in protected call")
)

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
	probing          bool // half-open: hay una llamada de prueba en curso
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

// Execute ejecuta fn si el circuito lo permite. Un panic en fn cuenta como
// fallo (y se propaga), para no dejar el breaker esperando una prueba que
// nunca termina.
func (cb *CircuitBreaker) Execute(fn func() error) (err error) {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	finished := false
	defer func() {
		if !finished {
			cb.recordResult(errPanicInCall)
		}
	}()

	err = fn()
	finished = true
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
			cb.probing = true
			return true
		}
		return false
	case stateHalfOpen:
		// Una sola llamada de prueba a la vez: el resto falla rápido.
		if cb.probing {
			return false
		}
		cb.probing = true
		return true
	}
	return false
}

func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.probing = false
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