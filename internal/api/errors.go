package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// ErrorEnvelope es el cuerpo de error consistente de la API interna.
type ErrorEnvelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// errorMappings es la fuente única de verdad: error de dominio → (HTTP status, code).
// Añadir un error de dominio al contrato = añadir una fila; el orden define la precedencia.
var errorMappings = []struct {
	is     error
	status int
	code   string
}{
	{domain.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
	{domain.ErrDuplicateChecksum, http.StatusConflict, "DUPLICATE_CHECKSUM"},
	{domain.ErrDuplicateDocumentID, http.StatusConflict, "DUPLICATE_DOCUMENT_ID"},
	{domain.ErrRestoreConflict, http.StatusConflict, "RESTORE_CONFLICT"},
	{domain.ErrSummaryNotReady, http.StatusConflict, "SUMMARY_NOT_READY"},
}

func httpStatusFor(err error) int {
	for _, m := range errorMappings {
		if errors.Is(err, m.is) {
			return m.status
		}
	}
	return http.StatusInternalServerError
}

func errorCodeFor(err error) string {
	for _, m := range errorMappings {
		if errors.Is(err, m.is) {
			return m.code
		}
	}
	return "INTERNAL_ERROR"
}

// abortWithError escribe el envelope y corta el request.
func abortWithError(c *gin.Context, err error) {
	status := httpStatusFor(err)
	c.AbortWithStatusJSON(status, ErrorEnvelope{
		Code:    errorCodeFor(err),
		Message: err.Error(),
	})
}
