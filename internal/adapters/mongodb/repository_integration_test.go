//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestRepository_Integration_InsertAndFindByDocumentID(t *testing.T) {
	repo := newIntegrationRepo(t)

	doc := integrationDoc("doc-1", "a.pdf", "PENDING", time.Now().UTC())
	require.NoError(t, repo.Insert(context.Background(), doc))

	got, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	testutil.RequireSameContent(t, doc, *got)
	require.Nil(t, got.Summary)
	require.Equal(t, doc.Metadata, got.Metadata)
	require.Equal(t, doc.CreatedAt, got.CreatedAt)
}

func TestRepository_Integration_FindByDocumentID_NotFound(t *testing.T) {
	repo := newIntegrationRepo(t)

	_, err := repo.FindByDocumentID(context.Background(), "missing")

	require.Equal(t, domain.ErrNotFound, err)
}

func TestRepository_Integration_FindByChecksum(t *testing.T) {
	repo := newIntegrationRepo(t)

	insertDoc(t, repo, integrationDoc("doc-1", "a.pdf", "PENDING", time.Now().UTC()))

	got, err := repo.FindByChecksum(context.Background(), "chk-doc-1")
	require.NoError(t, err)
	require.Equal(t, "doc-1", got.DocumentID)

	_, err = repo.FindByChecksum(context.Background(), "chk-missing")
	require.Equal(t, domain.ErrNotFound, err)
}

func TestRepository_Integration_Complete(t *testing.T) {
	repo := newIntegrationRepo(t)

	created := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("doc-1", "a.pdf", "PENDING", created))

	summary := "resumen"
	got, err := repo.Complete(context.Background(), "doc-1", summary, 120)
	require.NoError(t, err)
	require.Equal(t, domain.SummaryResolved, got.SummaryStatus)
	require.Equal(t, &summary, got.Summary)
	require.Equal(t, int64(120), got.SummaryTimeMS)
	require.False(t, got.UpdatedAt.Before(created))

	persisted, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.SummaryResolved, persisted.SummaryStatus)
	require.Equal(t, &summary, persisted.Summary)
	require.Equal(t, int64(120), persisted.SummaryTimeMS)
}

func TestRepository_Integration_Complete_NotFound(t *testing.T) {
	repo := newIntegrationRepo(t)

	_, err := repo.Complete(context.Background(), "missing", "resumen", 0)

	require.Equal(t, domain.ErrNotFound, err)
}

func TestRepository_Integration_DuplicateDocumentID(t *testing.T) {
	repo := newIntegrationRepo(t)
	require.NoError(t, repo.EnsureIndexes(context.Background()))
	require.NoError(t, repo.EnsureIndexes(context.Background()))

	insertDoc(t, repo, integrationDoc("doc-1", "a.pdf", "PENDING", time.Now().UTC()))

	err := repo.Insert(context.Background(), integrationDoc("doc-1", "otro.pdf", "PENDING", time.Now().UTC()))
	require.Equal(t, domain.ErrDuplicateDocumentID, err)
}

// TestRepository_Integration_Restore_RaceDuplicateChecksum reproduce la
// carrera TOCTOU entre restoreConflictCheck y clearDeletedAt
func TestRepository_Integration_Restore_RaceDuplicateChecksum(t *testing.T) {
	repo := newIntegrationRepo(t)
	require.NoError(t, repo.EnsureIndexes(context.Background()))

	ctx := context.Background()
	chk := "chk-race-1"

	// doc-1 activo con checksum X
	doc1 := testutil.NewDoc("doc-1", "a.pdf", chk, domain.SummaryPending, time.Now().UTC().Truncate(time.Millisecond))
	insertDoc(t, repo, doc1)

	// SoftDelete: doc-1 sale del índice parcial (deleted_at seteado)
	require.NoError(t, repo.SoftDelete(ctx, "doc-1"))

	// doc-2 activo con el mismo checksum X (entra al índice parcial)
	doc2 := testutil.NewDoc("doc-2", "b.pdf", chk, domain.SummaryPending, time.Now().UTC().Truncate(time.Millisecond))
	insertDoc(t, repo, doc2)

	// clearDeletedAt(doc-1) simula el interleave: check pasó antes de que doc-2 ocupara el checksum
	// ahora el $unset deleted_at dispara 11000 en uq_checksum_active
	err := repo.clearDeletedAt(ctx, "doc-1")
	require.Equal(t, domain.ErrRestoreConflict, err)
}
