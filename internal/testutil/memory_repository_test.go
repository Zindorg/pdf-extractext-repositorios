package testutil

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMemoryRepository_SoftDelete_ReleaseChecksumForReingest(t *testing.T) {
	repo := NewMemoryRepository()

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

func TestMemoryRepository_SoftDelete_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	err := repo.SoftDelete(context.Background(), "no-existe")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestMemoryRepository_Restore_Success(t *testing.T) {
	repo := NewMemoryRepository()

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

func TestMemoryRepository_Restore_NotFound(t *testing.T) {
	repo := NewMemoryRepository()

	err := repo.Restore(context.Background(), "no-existe")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestMemoryRepository_Restore_AlreadyActive(t *testing.T) {
	repo := NewMemoryRepository()

	require.NoError(t, repo.Insert(context.Background(), domain.Document{
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "contenido",
	}))
	// Restaurar un doc ya activo → éxito idempotente
	require.NoError(t, repo.Restore(context.Background(), "doc-1"))
}

func TestMemoryRepository_Restore_ConflictChecksum(t *testing.T) {
	repo := NewMemoryRepository()

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
