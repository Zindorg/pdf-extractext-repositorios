package testutil

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

// DocumentLister es el contrato mínimo para listar documentos. Lo satisfacen
// el puerto de persistencia y el servicio de aplicación, de modo que los
// tests de listado usan el mismo helper contra cualquiera de los dos.
type DocumentLister interface {
	List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error)
}

// ListDocs lista los documentos del filtro indicado en la primera página y
// devuelve los items junto con el total de coincidencias.
func ListDocs(t *testing.T, lister DocumentLister, filter domain.ListFilter) ([]domain.Document, int64) {
	t.Helper()
	docs, total, err := lister.List(context.Background(), filter, 1, domain.DefaultPageSize)
	require.NoError(t, err)
	return docs, total
}

// RequireAllStatus verifica que todos los documentos listados tienen el
// estado de resumen esperado.
func RequireAllStatus(t *testing.T, docs []domain.Document, want domain.SummaryStatus) {
	t.Helper()
	for _, doc := range docs {
		require.Equal(t, want, doc.SummaryStatus)
	}
}
