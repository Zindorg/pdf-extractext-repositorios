package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

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
	c.JSON(http.StatusOK, toResponse(*doc))
}

func (h *DocumentHandler) GetByChecksum(c *gin.Context) {
	doc, err := h.service.GetByChecksum(c.Request.Context(), c.Param("checksum"))
	if err != nil {
		abortWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(*doc))
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
	items, total, err := h.service.List(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		abortWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse(items, total, page, pageSize))
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
	if !doc.IsCompleted() {
		abortWithError(c, domain.ErrSummaryPending)
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
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// serveText escribe un adjunto text/plain con Content-Disposition RFC 5987.
func (h *DocumentHandler) serveText(c *gin.Context, text, filename string) {
	c.Header("Content-Disposition", contentDisposition(filename))
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(text))
}

// contentDisposition construye el header de adjunto con nombre no-ASCII
// mediante el encoding filename*=UTF-8” (RFC 5987).
func contentDisposition(filename string) string {
	return "attachment; filename*=UTF-8''" + url.PathEscape(filename)
}

func downloadFilename(doc domain.Document) string {
	if doc.Metadata.Filename != "" {
		return doc.Metadata.Filename
	}
	return "document-" + doc.DocumentID + ".txt"
}

func listResponse(items []domain.Document, total int64, page, pageSize int) ListResponse {
	resp := make([]DocumentResponse, len(items))
	for i, doc := range items {
		resp[i] = toResponse(doc)
	}
	return ListResponse{Items: resp, Total: total, Page: page, PageSize: pageSize}
}

// parseListFilter valida y construye el ListFilter desde la query string.
func parseListFilter(c *gin.Context) (domain.ListFilter, error) {
	filter := domain.ListFilter{}
	var err error
	if filter.Status, err = parseStatus(c.Query("status")); err != nil {
		return filter, err
	}
	if s := c.Query("filename"); s != "" {
		filter.Filename = &s
	}
	if filter.CreatedFrom, err = parseTimeQuery(c.Query("created_from"), "created_from"); err != nil {
		return filter, err
	}
	if filter.CreatedTo, err = parseTimeQuery(c.Query("created_to"), "created_to"); err != nil {
		return filter, err
	}
	filter.IncludeDeleted = c.Query("include_deleted") == "true"
	return filter, nil
}

func parseStatus(raw string) (*domain.Status, error) {
	if raw == "" {
		return nil, nil
	}
	switch domain.Status(raw) {
	case domain.StatusPending, domain.StatusCompleted:
		s := domain.Status(raw)
		return &s, nil
	default:
		return nil, fmt.Errorf("invalid status")
	}
}

func parseTimeQuery(raw, name string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", name)
	}
	return &t, nil
}

func parsePagination(c *gin.Context) (int, int, error) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid page")
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(domain.DefaultPageSize)))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid page_size")
	}
	return page, pageSize, nil
}

// abortBadRequest responde 400 con code BAD_REQUEST para errores de
// validación HTTP (no son errores de dominio: no pasan por errorMappings).
func abortBadRequest(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusBadRequest, ErrorEnvelope{
		Code:    "BAD_REQUEST",
		Message: err.Error(),
	})
}
