package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDocument_IsDeleted(t *testing.T) {
	deletedAt := time.Now().UTC()

	require.True(t, (Document{DeletedAt: &deletedAt}).IsDeleted())
	require.False(t, (Document{}).IsDeleted())
}

func TestDocument_HasSummary(t *testing.T) {
	summary := "resumen"

	require.True(t, (Document{Summary: &summary}).HasSummary())
	require.False(t, (Document{}).HasSummary())
}

func TestSummaryStatus_Values(t *testing.T) {
	require.Equal(t, "NOT_REQUESTED", string(SummaryNotRequested))
	require.Equal(t, "PENDING", string(SummaryPending))
	require.Equal(t, "RESOLVED", string(SummaryResolved))
	require.Equal(t, "FAILED", string(SummaryFailed))
}
