package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/application"
)

// HealthChecker abstrae el health de Mongo y Redis hacia el handler.
type HealthChecker interface {
	Ping() error
}

// DocumentHandler expone los endpoints internos de lectura/borrado.
// En esta fase responde 501 para toda la lógica pendiente; /health
// ya verifica dependencias reales.
type DocumentHandler struct {
	service *application.DocumentService
	health  HealthChecker
}

func NewDocumentHandler(service *application.DocumentService, health HealthChecker) *DocumentHandler {
	return &DocumentHandler{service: service, health: health}
}

func (h *DocumentHandler) GetByDocumentID(c *gin.Context) {
	_, err := h.service.GetByDocumentID(c.Request.Context(), c.Param("document_id"))
	if err != nil {
		abortWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, nil)
}

func (h *DocumentHandler) GetByChecksum(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}

func (h *DocumentHandler) List(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}

func (h *DocumentHandler) DownloadOriginal(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}

func (h *DocumentHandler) DownloadSummary(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}

func (h *DocumentHandler) SoftDelete(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}

func (h *DocumentHandler) Restore(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}

func (h *DocumentHandler) Health(c *gin.Context) {
	// TODO(TDD): fase roja — se restaura el ping a Mongo/Redis en la fase de lógica.
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}
