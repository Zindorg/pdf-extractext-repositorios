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

func TestSoftDelete_Success(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "chk-1", "contenido")

	require.NoError(t, svc.SoftDelete(context.Background(), "doc-1"))

	// Borrado lógico → invisible para lectura
	_, err := svc.GetByDocumentID(context.Background(), "doc-1")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSoftDelete_NotFound(t *testing.T) {
	svc, _ := newService()

	err := svc.SoftDelete(context.Background(), "no-existe")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	svc, repo := newService()

	insertSoftDeleted(t, repo, domain.Document{DocumentID: "doc-1", Checksum: "chk-1"})

	// Borrar algo ya borrado → éxito idempotente
	require.NoError(t, svc.SoftDelete(context.Background(), "doc-1"))
}

func TestRestore_Success(t *testing.T) {
	svc, repo := newService()

	insertSoftDeleted(t, repo, domain.Document{DocumentID: "doc-1", Checksum: "chk-1"})

	require.NoError(t, svc.Restore(context.Background(), "doc-1"))

	doc, err := svc.GetByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.False(t, doc.IsDeleted())
}

func TestRestore_NotFound(t *testing.T) {
	svc, _ := newService()

	err := svc.Restore(context.Background(), "no-existe")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRestore_AlreadyActive(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "chk-1", "")

	// Restaurar un doc activo → éxito idempotente
	require.NoError(t, svc.Restore(context.Background(), "doc-1"))
}

func TestRestore_ConflictChecksum(t *testing.T) {
	svc, repo := newService()

	// Doc A: borrado, checksum "chk-1"
	insertSoftDeleted(t, repo, domain.Document{DocumentID: "doc-A", Checksum: "chk-1"})
	// Doc B: ACTIVO, mismo checksum (re-ingest tras liberación)
	testutil.SeedDoc(t, repo, domain.Document{
		DocumentID: "doc-B", Checksum: "chk-1", ExtractedText: "b",
	})

	err := svc.Restore(context.Background(), "doc-A")
	require.ErrorIs(t, err, domain.ErrRestoreConflict)

	// doc-A sigue borrado
	docA, _ := repo.FindByDocumentID(context.Background(), "doc-A")
	require.True(t, docA.IsDeleted())
}

func TestCreatePending_StoresPendingDocument(t *testing.T) {
	svc, repo := newService()

	payload := domain.Document{
		DocumentID:       "doc-1",
		Checksum:         "abc123",
		SummaryStatus:    domain.SummaryPending,
		ExtractedText:    "contenido extraído",
		ExtractionTimeMS: 40,
		SummaryTimeMS:    2,
		Metadata: domain.Metadata{
			Filename:  "informe.pdf",
			MimeType:  "application/pdf",
			SizeBytes: 2048576,
			PageCount: 12,
		},
	}

	got, err := svc.CreatePending(context.Background(), payload)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.SummaryPending, got.SummaryStatus)
	require.False(t, got.CreatedAt.IsZero())
	require.False(t, got.IsDeleted())

	stored, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, payload.ExtractedText, stored.ExtractedText)
	require.Equal(t, payload.Checksum, stored.Checksum)
	require.Equal(t, payload.Metadata, stored.Metadata)
	require.Equal(t, payload.ExtractionTimeMS, stored.ExtractionTimeMS)
	require.Equal(t, payload.SummaryTimeMS, stored.SummaryTimeMS)
}

func TestCreatePending_DuplicateChecksum(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "same-checksum", "a")

	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID: "doc-2", Checksum: "same-checksum", ExtractedText: "b",
	})
	require.ErrorIs(t, err, domain.ErrDuplicateChecksum)
}

func TestCreatePending_DuplicateDocumentID(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "checksum-a", "")

	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "checksum-a",
	})
	require.ErrorIs(t, err, domain.ErrDuplicateDocumentID)
}

func TestCompleteWithSummary_CompletesPendingDocument(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "abc123", "contenido original")

	got, err := svc.CompleteWithSummary(context.Background(), "doc-1", "resumen del doc", 120)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.SummaryResolved, got.SummaryStatus)
	require.NotNil(t, got.Summary)
	require.Equal(t, "resumen del doc", *got.Summary)
	require.Equal(t, int64(120), got.SummaryTimeMS)
	require.True(t, got.UpdatedAt.After(got.CreatedAt))
}

func TestCompleteWithSummary_OrphanSummary_ReturnsErrNotFound(t *testing.T) {
	svc, _ := newService()

	_, err := svc.CompleteWithSummary(context.Background(), "no-existe", "resumen huérfano", 120)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCompleteWithSummary_AlreadyCompleted_IsBenign(t *testing.T) {
	svc, repo := newService()

	seedPending(t, svc, "doc-1", "abc123", "")

	_, err := svc.CompleteWithSummary(context.Background(), "doc-1", "primera vez", 120)
	require.NoError(t, err)

	// Duplicado legítimo (entrega at-least-once): sin error → consumer hace XACK + descarte.
	_, err = svc.CompleteWithSummary(context.Background(), "doc-1", "otra vez", 200)
	require.NoError(t, err)

	stored, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.SummaryResolved, stored.SummaryStatus)
	require.Equal(t, "primera vez", *stored.Summary)
	require.Equal(t, int64(120), stored.SummaryTimeMS)
}

func TestGetByDocumentID_Found(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "abc123", "contenido")

	got, err := svc.GetByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, domain.SummaryPending, got.SummaryStatus)
	require.Equal(t, "contenido", got.ExtractedText)
}

func TestGetByDocumentID_NotFound(t *testing.T) {
	svc, _ := newService()

	got, err := svc.GetByDocumentID(context.Background(), "no-existe")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetByDocumentID_SoftDeleted_ReturnsErrNotFound(t *testing.T) {
	svc, repo := newService()

	insertSoftDeleted(t, repo, domain.Document{DocumentID: "doc-del", Checksum: "del-1"})

	got, err := svc.GetByDocumentID(context.Background(), "doc-del")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetByChecksum_Found(t *testing.T) {
	svc, _ := newService()

	seedPending(t, svc, "doc-1", "abc123", "contenido")

	got, err := svc.GetByChecksum(context.Background(), "abc123")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, "abc123", got.Checksum)
}

func TestGetByChecksum_NotFound(t *testing.T) {
	svc, _ := newService()

	got, err := svc.GetByChecksum(context.Background(), "no-existe")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetByChecksum_SoftDeleted_ReturnsErrNotFound(t *testing.T) {
	svc, repo := newService()

	insertSoftDeleted(t, repo, domain.Document{DocumentID: "doc-del", Checksum: "del-check"})

	got, err := svc.GetByChecksum(context.Background(), "del-check")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestList_DefaultPagination(t *testing.T) {
	svc, repo := newService()

	seedListDocs(t, repo, 25)

	docs, total, err := svc.List(context.Background(), domain.ListFilter{}, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(25), total)
	require.Len(t, docs, 20) // default pageSize=20
	// Verifica orden: created_at DESC (más reciente primero)
	require.True(t, docs[0].CreatedAt.After(docs[1].CreatedAt))
}

func TestList_CustomPagination(t *testing.T) {
	svc, repo := newService()

	seedListDocs(t, repo, 25)

	// page=2, pageSize=10
	docs, total, err := svc.List(context.Background(), domain.ListFilter{}, 2, 10)
	require.NoError(t, err)
	require.Equal(t, int64(25), total)
	require.Len(t, docs, 10)

	// Clamp max pageSize=100
	docs, _, err = svc.List(context.Background(), domain.ListFilter{}, 1, 150)
	require.NoError(t, err)
	require.Len(t, docs, 25) // clamp a 100, pero solo hay 25 docs
}

func TestList_FilterByStatus(t *testing.T) {
	svc, repo := newService()

	testutil.SeedDoc(t, repo, makeListDoc("doc-1", "PENDING"))
	testutil.SeedDoc(t, repo, makeListDoc("doc-2", "RESOLVED"))
	testutil.SeedDoc(t, repo, makeListDoc("doc-3", "PENDING"))

	pending := domain.SummaryPending
	docs, total := testutil.ListDocs(t, svc, domain.ListFilter{SummaryStatus: &pending})

	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
	testutil.RequireAllStatus(t, docs, domain.SummaryPending)
}

func TestList_FilterByFilename(t *testing.T) {
	svc, repo := newService()

	testutil.SeedDoc(t, repo, testutil.NewListDoc("doc-1", "informe.pdf", domain.SummaryPending, time.Time{}))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("doc-2", "factura.pdf", domain.SummaryPending, time.Time{}))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("doc-3", "INFORME_anual.pdf", domain.SummaryPending, time.Time{}))

	fn := "informe"
	_, total := testutil.ListDocs(t, svc, domain.ListFilter{Filename: &fn})

	require.Equal(t, int64(2), total) // "informe.pdf" + "INFORME_anual.pdf" (case-insensitive)
}

func TestList_FilterByCreatedRange(t *testing.T) {
	svc, repo := newService()

	now := time.Now().UTC()
	testutil.SeedDoc(t, repo, testutil.NewListDoc("old", "", domain.SummaryPending, now.Add(-48*time.Hour)))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("mid", "", domain.SummaryPending, now.Add(-24*time.Hour)))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("new", "", domain.SummaryPending, now.Add(-1*time.Hour)))

	from := now.Add(-36 * time.Hour)
	to := now.Add(-12 * time.Hour)
	docs, total := testutil.ListDocs(t, svc, domain.ListFilter{CreatedFrom: &from, CreatedTo: &to})

	require.Equal(t, int64(1), total) // solo "mid"
	require.Equal(t, "mid", docs[0].DocumentID)
}

func TestList_IncludeDeleted(t *testing.T) {
	svc, repo := newService()

	testutil.SeedDoc(t, repo, makeListDoc("active", "PENDING"))
	insertSoftDeleted(t, repo, makeListDoc("deleted", "PENDING"))

	// Sin IncludeDeleted → excluye borrados
	docs, _ := testutil.ListDocs(t, svc, domain.ListFilter{})
	require.Len(t, docs, 1)
	require.Equal(t, "active", docs[0].DocumentID)

	// Con IncludeDeleted=true → incluye borrados
	docs, total := testutil.ListDocs(t, svc, domain.ListFilter{IncludeDeleted: true})
	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
}

func TestList_ExcludesDeletedByDefault(t *testing.T) {
	svc, repo := newService()

	insertSoftDeleted(t, repo, makeListDoc("deleted", "PENDING"))

	_, total := testutil.ListDocs(t, svc, domain.ListFilter{})
	require.Equal(t, int64(0), total)
}

func TestList_ReturnsTotal(t *testing.T) {
	svc, repo := newService()

	seedListDocs(t, repo, 42)

	_, total, err := svc.List(context.Background(), domain.ListFilter{}, 2, 10)
	require.NoError(t, err)
	require.Equal(t, int64(42), total) // total correcto para paginación
}

// seedListDocs inserta n documentos PENDING con ids "doc-1".."doc-n".
func seedListDocs(t *testing.T, repo *testutil.MemoryRepository, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		testutil.SeedDoc(t, repo, makeListDoc(fmt.Sprintf("doc-%d", i), "PENDING"))
	}
}
