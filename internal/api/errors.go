package api

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// ErrorEnvelope es el cuerpo de error consistente de la API interna.
type ErrorEnvelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ErrorCode del dominio → HTTP.
func httpStatusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return 404
	case errors.Is(err, domain.ErrDuplicateChecksum),
		errors.Is(err, domain.ErrDuplicateDocumentID),
		errors.Is(err, domain.ErrRestoreConflict):
		return 409
	case errors.Is(err, domain.ErrSummaryNotReady):
		return 409
	default:
		return 500
	}
}

func errorCodeFor(err error) string {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrDuplicateChecksum):
		return "DUPLICATE_CHECKSUM"
	case errors.Is(err, domain.ErrDuplicateDocumentID):
		return "DUPLICATE_DOCUMENT_ID"
	case errors.Is(err, domain.ErrRestoreConflict):
		return "RESTORE_CONFLICT"
	case errors.Is(err, domain.ErrSummaryNotReady):
		return "SUMMARY_NOT_READY"
	default:
		return "INTERNAL_ERROR"
	}
}

// abortWithError escribe el envelope y corta el request.
func abortWithError(c *gin.Context, err error) {
	status := httpStatusFor(err)
	c.AbortWithStatusJSON(status, ErrorEnvelope{
		Code:    errorCodeFor(err),
		Message: err.Error(),
	})
}
