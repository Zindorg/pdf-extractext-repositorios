package redis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRetryDLQ(t *testing.T) {
	d := NewRetryDLQ(3, 2*time.Second)

	require.Equal(t, 3, d.MaxRetries())
	require.Equal(t, 2*time.Second, d.Backoff())
}

func TestNeedsDLQ(t *testing.T) {
	d := NewRetryDLQ(3, time.Second)

	require.False(t, d.NeedsDLQ(0))
	require.False(t, d.NeedsDLQ(2))
	require.True(t, d.NeedsDLQ(3))
	require.True(t, d.NeedsDLQ(4))
}
