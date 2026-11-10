// Package testutil centraliza fixtures de documentos compartidas por los
// tests unitarios (application) y los de integración (mongodb), para que
// ambos niveles prueben exactamente la misma forma de documento.
package testutil

import (
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

// NewDoc crea un documento PDF válido mínimo para tests.
func NewDoc(id, filename, checksum string, status domain.SummaryStatus, created time.Time) domain.Document {
	return domain.Document{
		DocumentID:    id,
		Checksum:      checksum,
		SummaryStatus: status,
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

// NewListDoc crea un documento para tests de listado: el checksum se deriva
// del id ("chk-"+id), convención que comparten todos los niveles de test.
func NewListDoc(id, filename string, status domain.SummaryStatus, created time.Time) domain.Document {
	return NewDoc(id, filename, "chk-"+id, status, created)
}

// CompleteDoc crea un documento con todos los campos poblados (resumen
// incluido) para los tests de mapeo de persistencia y de DTO.
func CompleteDoc() domain.Document {
	summary := "resumen"
	created := time.Now().UTC()
	return domain.Document{
		DocumentID:       "doc-1",
		Checksum:         "abc",
		SummaryStatus:    domain.SummaryResolved,
		ExtractedText:    "texto",
		Summary:          &summary,
		Metadata:         domain.Metadata{Filename: "x.pdf", MimeType: "application/pdf", SizeBytes: 10, PageCount: 2},
		ExtractionTimeMS: 3,
		SummaryTimeMS:    2,
		CreatedAt:        created,
		UpdatedAt:        created,
	}
}

// WithDeleted devuelve una copia de doc marcada como soft-delete.
func WithDeleted(doc domain.Document, deletedAt time.Time) domain.Document {
	doc.DeletedAt = &deletedAt
	return doc
}

// RequireSameContent verifica que el documento persistido conserva la
// identidad y el texto extraído del original.
func RequireSameContent(t *testing.T, want, got domain.Document) {
	t.Helper()
	require.Equal(t, want.DocumentID, got.DocumentID)
	require.Equal(t, want.Checksum, got.Checksum)
	require.Equal(t, want.SummaryStatus, got.SummaryStatus)
	require.Equal(t, want.ExtractedText, got.ExtractedText)
}

// RequireSummaryResolved verifica que el documento quedó resuelto con el
// resumen y la duración esperados.
func RequireSummaryResolved(t *testing.T, doc domain.Document, summary string, timeMS int64) {
	t.Helper()
	require.Equal(t, domain.SummaryResolved, doc.SummaryStatus)
	require.NotNil(t, doc.Summary)
	require.Equal(t, summary, *doc.Summary)
	require.Equal(t, timeMS, doc.SummaryTimeMS)
}
