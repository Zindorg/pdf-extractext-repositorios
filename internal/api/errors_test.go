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
		{name: "duplicate document id", err: domain.ErrDuplicateDocumentID, want: http.StatusConflict},
		{name: "restore conflict", err: domain.ErrRestoreConflict, want: http.StatusConflict},
		{name: "summary not ready", err: domain.ErrSummaryNotReady, want: http.StatusConflict},
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
		{name: "not found", err: domain.ErrNotFound, want: "NOT_FOUND"},
		{name: "duplicate checksum", err: domain.ErrDuplicateChecksum, want: "DUPLICATE_CHECKSUM"},
		{name: "duplicate document id", err: domain.ErrDuplicateDocumentID, want: "DUPLICATE_DOCUMENT_ID"},
		{name: "restore conflict", err: domain.ErrRestoreConflict, want: "RESTORE_CONFLICT"},
		{name: "summary not ready", err: domain.ErrSummaryNotReady, want: "SUMMARY_NOT_READY"},
		{name: "wrapped domain error", err: fmt.Errorf("capa: %w", domain.ErrNotFound), want: "NOT_FOUND"},
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
	require.JSONEq(t, `{"code":"NOT_FOUND","message":"document not found"}`, rr.Body.String())
}
