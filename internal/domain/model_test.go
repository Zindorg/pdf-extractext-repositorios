package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDocument_IsCompleted(t *testing.T) {
	require.True(t, (Document{Status: StatusCompleted}).IsCompleted())
	require.False(t, (Document{Status: StatusPending}).IsCompleted())
	require.False(t, (Document{}).IsCompleted())
}

func TestDocument_IsDeleted(t *testing.T) {
	deletedAt := time.Now().UTC()

	require.True(t, (Document{DeletedAt: &deletedAt}).IsDeleted())
	require.False(t, (Document{}).IsDeleted())
}
