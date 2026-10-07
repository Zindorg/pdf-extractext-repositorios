package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// HealthChecker abstrae el health de Mongo y Redis hacia el handler.
type HealthChecker interface {
	Ping() error
}

// DocumentUseCases es el puerto que api consume de la capa de aplicación.
// Interfaz definida por el consumidor (DIP): el handler no depende del
// tipo concreto *application.DocumentService, solo de lo que usa.
type DocumentUseCases interface {
	GetByDocumentID(ctx context.Context, documentID string) (*domain.Document, error)
	GetByChecksum(ctx context.Context, checksum string) (*domain.Document, error)
	List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error)
	SoftDelete(ctx context.Context, documentID string) error
	Restore(ctx context.Context, documentID string) error
}

// DocumentHandler expone los endpoints internos de lectura/borrado.
// En esta fase responde 501 para toda la lógica pendiente; /health
// ya verifica dependencias reales.
type DocumentHandler struct {
	service DocumentUseCases
	health  HealthChecker
}

func NewDocumentHandler(service DocumentUseCases, health HealthChecker) *DocumentHandler {
	return &DocumentHandler{service: service, health: health}
}

func (h *DocumentHandler) GetByDocumentID(c *gin.Context) {
	notImplemented(c)
}

func (h *DocumentHandler) GetByChecksum(c *gin.Context) {
	notImplemented(c)
}

func (h *DocumentHandler) List(c *gin.Context) {
	notImplemented(c)
}

func (h *DocumentHandler) DownloadOriginal(c *gin.Context) {
	notImplemented(c)
}

func (h *DocumentHandler) DownloadSummary(c *gin.Context) {
	notImplemented(c)
}

func (h *DocumentHandler) SoftDelete(c *gin.Context) {
	notImplemented(c)
}

func (h *DocumentHandler) Restore(c *gin.Context) {
	notImplemented(c)
}

// notImplemented responde el envelope 501 de la fase de lógica pendiente.
func notImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"code": "NOT_IMPLEMENTED", "message": "fase de lógica pendiente"})
}

func (h *DocumentHandler) Health(c *gin.Context) {
	if h.health != nil {
		if err := h.health.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
