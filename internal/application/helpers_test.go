package application_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

// newService crea el servicio real sobre el fake en-memoria compartido
// (testutil), de modo que los tests ejerciten la lógica de aplicación.
func newService() (*application.DocumentService, *testutil.MemoryRepository) {
	repo := testutil.NewMemoryRepository()
	return application.NewDocumentService(repo), repo
}

// seedPending inserta un documento PENDING vía el servicio.
func seedPending(t *testing.T, svc *application.DocumentService, id, checksum, text string) {
	t.Helper()
	doc := testutil.NewDoc(id, "", checksum, domain.SummaryPending, time.Now().UTC())
	doc.ExtractedText = text
	_, err := svc.CreatePending(context.Background(), doc)
	require.NoError(t, err)
}

// insertSoftDeleted guarda un documento ya borrado (soft delete) vía el fake,
// imitando el estado que deja SoftDelete: borrado hace una hora.
func insertSoftDeleted(t *testing.T, repo *testutil.MemoryRepository, doc domain.Document) {
	t.Helper()
	testutil.SeedSoftDeleted(t, repo, doc, time.Now().UTC().Add(-time.Hour))
}

// makeListDoc crea un documento de prueba para tests de List sobre el
// fixture compartido (testutil.NewListDoc), fijando las convenciones locales:
// sin filename y created_at relativo a hace una hora.
func makeListDoc(id, status string) domain.Document {
	return testutil.NewListDoc(id, "", domain.SummaryStatus(status), time.Now().UTC().Add(-time.Hour))
}

// seedListDocs inserta n documentos PENDING con ids "doc-1".."doc-n".
func seedListDocs(t *testing.T, repo *testutil.MemoryRepository, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		testutil.SeedDoc(t, repo, makeListDoc(fmt.Sprintf("doc-%d", i), "PENDING"))
	}
}
