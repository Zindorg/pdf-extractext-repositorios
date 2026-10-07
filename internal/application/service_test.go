package application_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

// inMemoryRepo es un fake en-memoria del port domain.DocumentRepository:
// implementación real con dedup por document_id/checksum, posta para probar
// el DocumentService sin infraestructura (seam del caso de uso).
type inMemoryRepo struct {
	byID       map[string]domain.Document // document_id → documento
	checksumID map[string]string          // checksum → document_id (activos)
}

func newInMemoryRepo() *inMemoryRepo {
	return &inMemoryRepo{
		byID:       make(map[string]domain.Document),
		checksumID: make(map[string]string),
	}
}

func (r *inMemoryRepo) Insert(_ context.Context, doc domain.Document) error {
	if _, ok := r.byID[doc.DocumentID]; ok {
		return domain.ErrDuplicateDocumentID
	}
	if id, ok := r.checksumID[doc.Checksum]; ok && !r.byID[id].IsDeleted() {
		return domain.ErrDuplicateChecksum
	}
	r.byID[doc.DocumentID] = doc
	r.checksumID[doc.Checksum] = doc.DocumentID
	return nil
}

// UpdateStatus persiste el cambio de estado (fake fiel del port):
// inexistente → ErrNotFound (huérfano); existente → COMPLETED + updated_at.
func (r *inMemoryRepo) UpdateStatus(_ context.Context, documentID string, status domain.Status, summary *string) (*domain.Document, error) {
	doc, ok := r.byID[documentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	doc.Status = status
	doc.Summary = summary
	doc.UpdatedAt = domain.Now()
	r.byID[documentID] = doc
	return &doc, nil
}

func (r *inMemoryRepo) FindByDocumentID(_ context.Context, documentID string) (*domain.Document, error) {
	doc, ok := r.byID[documentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &doc, nil
}

func (r *inMemoryRepo) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	id, ok := r.checksumID[checksum]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.FindByDocumentID(ctx, id)
}

// List asume page/pageSize ya normalizados por el servicio (ver puerto).
func (r *inMemoryRepo) List(_ context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	var filtered []domain.Document
	for _, doc := range r.byID {
		// Filter by status
		if filter.Status != nil && doc.Status != *filter.Status {
			continue
		}
		// Filter by filename (case-insensitive contains)
		if filter.Filename != nil {
			filename := strings.ToLower(doc.Metadata.Filename)
			search := strings.ToLower(*filter.Filename)
			if !strings.Contains(filename, search) {
				continue
			}
		}
		// Filter by created range
		if filter.CreatedFrom != nil && doc.CreatedAt.Before(*filter.CreatedFrom) {
			continue
		}
		if filter.CreatedTo != nil && doc.CreatedAt.After(*filter.CreatedTo) {
			continue
		}
		// Soft delete filter
		if !filter.IncludeDeleted && doc.IsDeleted() {
			continue
		}
		filtered = append(filtered, doc)
	}

	// Sort by created_at DESC (newest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	total := int64(len(filtered))

	// Pagination
	start := (page - 1) * pageSize
	if start >= len(filtered) {
		return []domain.Document{}, total, nil
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (r *inMemoryRepo) SoftDelete(_ context.Context, documentID string) error {
	doc, ok := r.byID[documentID]
	if !ok {
		return domain.ErrNotFound
	}
	now := time.Now().UTC()
	doc.DeletedAt = &now
	doc.UpdatedAt = now
	r.byID[documentID] = doc
	delete(r.checksumID, doc.Checksum) // simula índice parcial: el checksum se libera
	return nil
}

func (r *inMemoryRepo) Restore(_ context.Context, _ string) error {
	return nil
}

func TestRepoSoftDelete_ReleaseChecksumForReingest(t *testing.T) {
	repo := newInMemoryRepo()

	require.NoError(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "mismo contenido",
	}))
	require.NoError(t, repo.SoftDelete(context.Background(), "doc-1"))

	// El soft delete libera el checksum (índice parcial): re-ingiriendo el
	// mismo contenido con un nuevo document_id debe funcionar.
	require.NoError(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-2", Checksum: "chk-1", ExtractedText: "mismo contenido",
	}))
}

func TestRepoSoftDelete_NotFound(t *testing.T) {
	repo := newInMemoryRepo()

	err := repo.SoftDelete(context.Background(), "no-existe")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRepoRestore_Success(t *testing.T) {
	repo := newInMemoryRepo()

	require.NoError(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "contenido",
	}))
	require.NoError(t, repo.SoftDelete(context.Background(), "doc-1"))
	require.NoError(t, repo.Restore(context.Background(), "doc-1"))

	// Restaurado: visible en Get y checksum reapropiado
	doc, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.False(t, doc.IsDeleted())
	// Re-ingestar el mismo checksum con OTRO id debe fallar (checksum reapropiado)
	require.ErrorIs(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-2", Checksum: "chk-1", ExtractedText: "contenido",
	}), domain.ErrDuplicateChecksum)
}

func TestRepoRestore_NotFound(t *testing.T) {
	repo := newInMemoryRepo()

	err := repo.Restore(context.Background(), "no-existe")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRepoRestore_AlreadyActive(t *testing.T) {
	repo := newInMemoryRepo()

	require.NoError(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "contenido",
	}))
	// Restaurar un doc ya activo → éxito idempotente
	require.NoError(t, repo.Restore(context.Background(), "doc-1"))
}

func TestRepoRestore_ConflictChecksum(t *testing.T) {
	repo := newInMemoryRepo()

	// Doc A: borrado, checksum "chk-1"
	require.NoError(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-A", Checksum: "chk-1", ExtractedText: "a",
	}))
	require.NoError(t, repo.SoftDelete(context.Background(), "doc-A"))

	// Doc B: ACTIVO, mismo checksum "chk-1"
	require.NoError(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-B", Checksum: "chk-1", ExtractedText: "b",
	}))

	// Restaurar doc-A → conflicto con doc-B activo
	err := repo.Restore(context.Background(), "doc-A")
	require.ErrorIs(t, err, domain.ErrRestoreConflict)

	// doc-A sigue borrado (no se restauró)
	docA, _ := repo.FindByDocumentID(context.Background(), "doc-A")
	require.True(t, docA.IsDeleted())
}

func TestCreatePending_StoresPendingDocument(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	payload := domain.Document{
		DocumentID:       "doc-1",
		Checksum:         "abc123",
		ExtractedText:    "contenido extraído",
		ProcessingTimeMS: 42,
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
	require.Equal(t, domain.StatusPending, got.Status)
	require.False(t, got.CreatedAt.IsZero())
	require.False(t, got.IsDeleted())

	stored, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, payload.ExtractedText, stored.ExtractedText)
	require.Equal(t, payload.Checksum, stored.Checksum)
	require.Equal(t, payload.Metadata, stored.Metadata)
	require.Equal(t, payload.ProcessingTimeMS, stored.ProcessingTimeMS)
}

func TestCreatePending_DuplicateChecksum(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	first := domain.Document{DocumentID: "doc-1", Checksum: "same-checksum", ExtractedText: "a"}
	_, err := svc.CreatePending(context.Background(), first)
	require.NoError(t, err)

	second := domain.Document{DocumentID: "doc-2", Checksum: "same-checksum", ExtractedText: "b"}
	_, err = svc.CreatePending(context.Background(), second)
	require.ErrorIs(t, err, domain.ErrDuplicateChecksum)
}

func TestCreatePending_DuplicateDocumentID(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	payload := domain.Document{DocumentID: "doc-1", Checksum: "checksum-a"}
	_, err := svc.CreatePending(context.Background(), payload)
	require.NoError(t, err)

	_, err = svc.CreatePending(context.Background(), payload)
	require.ErrorIs(t, err, domain.ErrDuplicateDocumentID)
}

func TestCompleteWithSummary_CompletesPendingDocument(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID:    "doc-1",
		Checksum:      "abc123",
		ExtractedText: "contenido original",
	})
	require.NoError(t, err)

	got, err := svc.CompleteWithSummary(context.Background(), "doc-1", "resumen del doc")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, domain.StatusCompleted, got.Status)
	require.NotNil(t, got.Summary)
	require.Equal(t, "resumen del doc", *got.Summary)
	require.True(t, got.UpdatedAt.After(got.CreatedAt))
}

func TestCompleteWithSummary_OrphanSummary_ReturnsErrNotFound(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	_, err := svc.CompleteWithSummary(context.Background(), "no-existe", "resumen huérfano")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCompleteWithSummary_AlreadyCompleted_IsBenign(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	_, err := svc.CreatePending(context.Background(), domain.Document{DocumentID: "doc-1", Checksum: "abc123"})
	require.NoError(t, err)

	_, err = svc.CompleteWithSummary(context.Background(), "doc-1", "primera vez")
	require.NoError(t, err)

	// Duplicado legítimo (entrega at-least-once): sin error → consumer hace XACK + descarte.
	_, err = svc.CompleteWithSummary(context.Background(), "doc-1", "otra vez")
	require.NoError(t, err)

	stored, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, stored.Status)
	require.Equal(t, "primera vez", *stored.Summary)
}

// insertSoftDeleted guarda un documento ya borrado (soft delete) vía el fake,
// imitando el estado que dejará SoftDelete cuando esté implementado.
func insertSoftDeleted(t *testing.T, repo *inMemoryRepo, doc domain.Document) {
	t.Helper()
	deletedAt := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, repo.Insert(context.Background(), testutil.WithDeleted(doc, deletedAt)))
}

func TestGetByDocumentID_Found(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "abc123", ExtractedText: "contenido",
	})
	require.NoError(t, err)

	got, err := svc.GetByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, domain.StatusPending, got.Status)
	require.Equal(t, "contenido", got.ExtractedText)
}

func TestGetByDocumentID_NotFound(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	got, err := svc.GetByDocumentID(context.Background(), "no-existe")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetByDocumentID_SoftDeleted_ReturnsErrNotFound(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	insertSoftDeleted(t, repo, domain.Document{DocumentID: "doc-del", Checksum: "del-1"})

	got, err := svc.GetByDocumentID(context.Background(), "doc-del")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetByChecksum_Found(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	_, err := svc.CreatePending(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "abc123", ExtractedText: "contenido",
	})
	require.NoError(t, err)

	got, err := svc.GetByChecksum(context.Background(), "abc123")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "doc-1", got.DocumentID)
	require.Equal(t, "abc123", got.Checksum)
}

func TestGetByChecksum_NotFound(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	got, err := svc.GetByChecksum(context.Background(), "no-existe")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetByChecksum_SoftDeleted_ReturnsErrNotFound(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	insertSoftDeleted(t, repo, domain.Document{DocumentID: "doc-del", Checksum: "del-check"})

	got, err := svc.GetByChecksum(context.Background(), "del-check")
	require.Nil(t, got)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

// makeListDoc crea un documento de prueba para tests de List sobre el
// fixture compartido (testutil.NewDoc), fijando las convenciones locales:
// sin filename y created_at relativo a hace una hora.
func makeListDoc(id, checksum, status string) domain.Document {
	return testutil.NewDoc(id, "", checksum, domain.Status(status), time.Now().UTC().Add(-time.Hour))
}

func TestList_DefaultPagination(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	for i := 1; i <= 25; i++ {
		repo.Insert(context.Background(), makeListDoc(fmt.Sprintf("doc-%d", i), fmt.Sprintf("chk-%d", i), "PENDING"))
	}

	docs, total, err := svc.List(context.Background(), domain.ListFilter{}, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(25), total)
	require.Len(t, docs, 20) // default pageSize=20
	// Verifica orden: created_at DESC (más reciente primero)
	require.True(t, docs[0].CreatedAt.After(docs[1].CreatedAt))
}

func TestList_CustomPagination(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	for i := 1; i <= 25; i++ {
		repo.Insert(context.Background(), makeListDoc(fmt.Sprintf("doc-%d", i), fmt.Sprintf("chk-%d", i), "PENDING"))
	}

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
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	repo.Insert(context.Background(), makeListDoc("doc-1", "chk-1", "PENDING"))
	repo.Insert(context.Background(), makeListDoc("doc-2", "chk-2", "COMPLETED"))
	repo.Insert(context.Background(), makeListDoc("doc-3", "chk-3", "PENDING"))

	pending := domain.StatusPending
	docs, total, err := svc.List(context.Background(), domain.ListFilter{Status: &pending}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
	for _, d := range docs {
		require.Equal(t, domain.StatusPending, d.Status)
	}
}

func TestList_FilterByFilename(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	repo.Insert(context.Background(), testutil.NewDoc("doc-1", "informe.pdf", "c1", domain.StatusPending, time.Time{}))
	repo.Insert(context.Background(), testutil.NewDoc("doc-2", "factura.pdf", "c2", domain.StatusPending, time.Time{}))
	repo.Insert(context.Background(), testutil.NewDoc("doc-3", "INFORME_anual.pdf", "c3", domain.StatusPending, time.Time{}))

	fn := "informe"
	_, total, err := svc.List(context.Background(), domain.ListFilter{Filename: &fn}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total) // "informe.pdf" + "INFORME_anual.pdf" (case-insensitive)
}

func TestList_FilterByCreatedRange(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	now := time.Now().UTC()
	repo.Insert(context.Background(), testutil.NewDoc("old", "", "c1", domain.StatusPending, now.Add(-48*time.Hour)))
	repo.Insert(context.Background(), testutil.NewDoc("mid", "", "c2", domain.StatusPending, now.Add(-24*time.Hour)))
	repo.Insert(context.Background(), testutil.NewDoc("new", "", "c3", domain.StatusPending, now.Add(-1*time.Hour)))

	from := now.Add(-36 * time.Hour)
	to := now.Add(-12 * time.Hour)
	docs, total, err := svc.List(context.Background(), domain.ListFilter{CreatedFrom: &from, CreatedTo: &to}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total) // solo "mid"
	require.Equal(t, "mid", docs[0].DocumentID)
}

func TestList_IncludeDeleted(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	repo.Insert(context.Background(), makeListDoc("active", "c1", "PENDING"))
	insertSoftDeleted(t, repo, makeListDoc("deleted", "c2", "PENDING"))

	// Sin IncludeDeleted → excluye borrados
	docs, _, err := svc.List(context.Background(), domain.ListFilter{}, 1, 20)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "active", docs[0].DocumentID)

	// Con IncludeDeleted=true → incluye borrados
	docs, total, err := svc.List(context.Background(), domain.ListFilter{IncludeDeleted: true}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
}

func TestList_ExcludesDeletedByDefault(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	insertSoftDeleted(t, repo, makeListDoc("deleted", "c1", "PENDING"))

	_, total, err := svc.List(context.Background(), domain.ListFilter{}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
}

func TestList_ReturnsTotal(t *testing.T) {
	repo := newInMemoryRepo()
	svc := application.NewDocumentService(repo)

	for i := 1; i <= 42; i++ {
		repo.Insert(context.Background(), makeListDoc(fmt.Sprintf("doc-%d", i), fmt.Sprintf("chk-%d", i), "PENDING"))
	}

	_, total, err := svc.List(context.Background(), domain.ListFilter{}, 2, 10)
	require.NoError(t, err)
	require.Equal(t, int64(42), total) // total correcto para paginación
}
