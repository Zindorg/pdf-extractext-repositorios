package api

import (
	"context"
	"log/slog"
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

// DocumentHandler expone los endpoints internos de lectura/escritura.
type DocumentHandler struct {
	service DocumentUseCases
	health  HealthChecker
}

func NewDocumentHandler(service DocumentUseCases, health HealthChecker) *DocumentHandler {
	return &DocumentHandler{service: service, health: health}
}

func (h *DocumentHandler) GetByDocumentID(c *gin.Context) {
	doc, err := h.service.GetByDocumentID(c.Request.Context(), c.Param("document_id"))
	if err != nil {
		abortWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(*doc, true))
}

func (h *DocumentHandler) GetByChecksum(c *gin.Context) {
	doc, err := h.service.GetByChecksum(c.Request.Context(), c.Param("checksum"))
	if err != nil {
		abortWithError(c, err)
		return
	}
	includeText := true
	if it := c.Query("include_text"); it != "" {
		includeText = it == "true"
	}
	c.JSON(http.StatusOK, ChecksumResponse{
		DocumentID:    doc.DocumentID,
		SummaryStatus: string(doc.SummaryStatus),
		Summary:       doc.Summary,
		ExtractedText: func() string {
			if includeText {
				return doc.ExtractedText
			}
			return ""
		}(),
	})
}

func (h *DocumentHandler) List(c *gin.Context) {
	filter, err := parseListFilter(c)
	if err != nil {
		abortBadRequest(c, err)
		return
	}
	page, pageSize, err := parsePagination(c)
	if err != nil {
		abortBadRequest(c, err)
		return
	}
	if filter.SummaryStatus != nil && *filter.SummaryStatus == domain.SummaryFailed {
		c.JSON(http.StatusOK, ListResponse{Items: []DocumentResponse{}, Total: 0, Page: page, PageSize: pageSize})
		return
	}
	items, total, err := h.service.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		abortWithError(c, err)
		return
	}
	includeText := true
	if filter.IncludeText != nil {
		includeText = *filter.IncludeText
	}
	c.JSON(http.StatusOK, listResponse(items, total, page, pageSize, includeText))
}

func (h *DocumentHandler) DownloadOriginal(c *gin.Context) {
	doc, err := h.service.GetByDocumentID(c.Request.Context(), c.Param("document_id"))
	if err != nil {
		abortWithError(c, err)
		return
	}
	h.serveText(c, doc.ExtractedText, downloadFilename(*doc))
}

func (h *DocumentHandler) DownloadSummary(c *gin.Context) {
	doc, err := h.service.GetByDocumentID(c.Request.Context(), c.Param("document_id"))
	if err != nil {
		abortWithError(c, err)
		return
	}
	if err := summaryError(*doc); err != nil {
		abortWithError(c, err)
		return
	}
	h.serveText(c, *doc.Summary, "summary-"+doc.DocumentID+".txt")
}

func (h *DocumentHandler) SoftDelete(c *gin.Context) {
	if err := h.service.SoftDelete(c.Request.Context(), c.Param("document_id")); err != nil {
		abortWithError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DocumentHandler) Restore(c *gin.Context) {
	if err := h.service.Restore(c.Request.Context(), c.Param("document_id")); err != nil {
		abortWithError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DocumentHandler) Health(c *gin.Context) {
	if h.health != nil {
		if err := h.health.Ping(); err != nil {
			slog.Error("health check failed", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
