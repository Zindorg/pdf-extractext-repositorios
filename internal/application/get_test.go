package application_test

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

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
