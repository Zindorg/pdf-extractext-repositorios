package application_test

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

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
