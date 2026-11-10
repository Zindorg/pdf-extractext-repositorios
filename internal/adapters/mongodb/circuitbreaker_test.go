package mongodb

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_ClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "closed", cb.State())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.Execute(func() error { return errors.New("fail") })
	}

	require.Equal(t, "open", cb.State())

	err := cb.Execute(func() error { return nil })
	require.ErrorIs(t, err, ErrCircuitOpen)
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	cb.Execute(func() error { return errors.New("fail") })
	cb.Execute(func() error { return errors.New("fail") })
	require.Equal(t, "open", cb.State())

	time.Sleep(60 * time.Millisecond)

	// State transitions to half-open on next Execute call
	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "half-open", cb.State())

	err = cb.Execute(func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "closed", cb.State())
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	cb.Execute(func() error { return errors.New("fail") })
	cb.Execute(func() error { return errors.New("fail") })

	time.Sleep(60 * time.Millisecond)
	// Trigger transition to half-open
	cb.Execute(func() error { return nil })
	require.Equal(t, "half-open", cb.State())

	err := cb.Execute(func() error { return errors.New("fail again") })
	require.Error(t, err)
	require.Equal(t, "open", cb.State())
}

func TestCircuitBreaker_ZeroThreshold_Defaults(t *testing.T) {
	cb := NewCircuitBreaker(0, 0, 0)
	require.Equal(t, 5, cb.failureThreshold)
	require.Equal(t, 2, cb.successThreshold)
	require.Equal(t, 30*time.Second, cb.timeout)
}

func TestCircuitBreaker_SuccessInClosedResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	cb.Execute(func() error { return errors.New("fail") })
	cb.Execute(func() error { return errors.New("fail") })
	require.Equal(t, 2, cb.failures)

	cb.Execute(func() error { return nil })
	require.Equal(t, 0, cb.failures)
}
