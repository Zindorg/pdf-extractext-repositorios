package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestHTTPStatusFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "not found", err: domain.ErrNotFound, want: http.StatusNotFound},
		{name: "duplicate checksum", err: domain.ErrDuplicateChecksum, want: http.StatusConflict},
		{name: "restore conflict", err: domain.ErrRestoreConflict, want: http.StatusConflict},
		{name: "summary not ready", err: domain.ErrSummaryPending, want: http.StatusConflict},
		{name: "wrapped domain error", err: fmt.Errorf("capa: %w", domain.ErrNotFound), want: http.StatusNotFound},
		{name: "root cause wins", err: fmt.Errorf("capa: %w", domain.ErrDuplicateChecksum), want: http.StatusConflict},
		{name: "unknown error", err: errors.New("boom"), want: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, httpStatusFor(tc.err))
		})
	}
}

func TestErrorCodeFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "not found", err: domain.ErrNotFound, want: "document_not_found"},
		{name: "duplicate checksum", err: domain.ErrDuplicateChecksum, want: "checksum_conflict"},
		{name: "restore conflict", err: domain.ErrRestoreConflict, want: "checksum_conflict"},
		{name: "summary not ready", err: domain.ErrSummaryPending, want: "summary_pending"},
		{name: "summary not available", err: domain.ErrSummaryNotAvailable, want: "summary_not_available"},
		{name: "unknown error", err: errors.New("boom"), want: "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, errorCodeFor(tc.err))
		})
	}
}

func TestAbortWithError(t *testing.T) {
	rr := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rr)

	abortWithError(c, domain.ErrNotFound)

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.JSONEq(t, `{"code":"document_not_found","message":"document not found"}`, rr.Body.String())
}
