//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRepository_Integration_InsertAndFindByDocumentID(t *testing.T) {
	repo := newIntegrationRepo(t)

	doc := integrationDoc("doc-1", "a.pdf", "PENDING", time.Now().UTC())
	require.NoError(t, repo.Insert(context.Background(), doc))

	got, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, doc.DocumentID, got.DocumentID)
	require.Equal(t, doc.Checksum, got.Checksum)
	require.Equal(t, doc.Status, got.Status)
	require.Equal(t, doc.ExtractedText, got.ExtractedText)
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

func TestRepository_Integration_UpdateStatus(t *testing.T) {
	repo := newIntegrationRepo(t)

	created := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("doc-1", "a.pdf", "PENDING", created))

	summary := "resumen"
	got, err := repo.UpdateStatus(context.Background(), "doc-1", domain.StatusCompleted, &summary, 120)
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, got.Status)
	require.Equal(t, &summary, got.Summary)
	require.Equal(t, int64(120), got.SummaryTimeMS)
	require.False(t, got.UpdatedAt.Before(created))

	persisted, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, persisted.Status)
	require.Equal(t, &summary, persisted.Summary)
	require.Equal(t, int64(120), persisted.SummaryTimeMS)
}

func TestRepository_Integration_UpdateStatus_NotFound(t *testing.T) {
	repo := newIntegrationRepo(t)

	_, err := repo.UpdateStatus(context.Background(), "missing", domain.StatusCompleted, nil, 0)

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
