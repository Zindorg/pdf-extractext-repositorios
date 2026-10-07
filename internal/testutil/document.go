// Package testutil centraliza fixtures de documentos compartidas por los
// tests unitarios (application) y los de integración (mongodb), para que
// ambos niveles prueben exactamente la misma forma de documento.
package testutil

import (
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// NewDoc crea un documento PDF válido mínimo para tests.
func NewDoc(id, filename, checksum string, status domain.Status, created time.Time) domain.Document {
	return domain.Document{
		DocumentID:    id,
		Checksum:      checksum,
		Status:        status,
		ExtractedText: "contenido",
		Metadata: domain.Metadata{
			Filename:  filename,
			MimeType:  "application/pdf",
			SizeBytes: 100,
			PageCount: 1,
		},
		CreatedAt: created,
		UpdatedAt: created,
	}
}

// WithDeleted devuelve una copia de doc marcada como soft-delete.
func WithDeleted(doc domain.Document, deletedAt time.Time) domain.Document {
	doc.DeletedAt = &deletedAt
	return doc
}
