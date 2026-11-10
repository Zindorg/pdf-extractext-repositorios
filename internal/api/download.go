package api

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

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

// summaryError separa "todavía no está" (409) de "no existe ni va a existir" (404).
func summaryError(doc domain.Document) error {
	switch {
	case doc.HasSummary():
		return nil
	case doc.SummaryStatus == domain.SummaryPending:
		return domain.ErrSummaryPending
	default:
		return domain.ErrSummaryNotAvailable
	}
}
