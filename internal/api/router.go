package api

import (
	"github.com/gin-gonic/gin"
)

// NewRouter construye el router Gin con middleware y el grupo /api/v1.
// No hay POST /documents: la creación llega solo por stream.
func NewRouter(handler *DocumentHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	v1 := r.Group("/api/v1")
	{
		v1.GET("/documents/:document_id", handler.GetByDocumentID)
		v1.GET("/documents/checksum/:checksum", handler.GetByChecksum)
		v1.GET("/documents", handler.List)
		v1.GET("/documents/:document_id/download/original", handler.DownloadOriginal)
		v1.GET("/documents/:document_id/download/summary", handler.DownloadSummary)
		v1.DELETE("/documents/:document_id", handler.SoftDelete)
		v1.POST("/documents/:document_id/restore", handler.Restore)
		v1.GET("/health", handler.Health)
	}

	return r
}
