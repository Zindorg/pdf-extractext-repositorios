package redis

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func newHandler() (*MessageHandlers, *testutil.MemoryRepository) {
	repo := testutil.NewMemoryRepository()
	service := application.NewDocumentService(repo)
	return NewMessageHandlers(service), repo
}

func TestOnOriginal_CreatesPendingDocument(t *testing.T) {
	h, repo := newHandler()

	err := h.OnOriginal(context.Background(), &OriginalEvent{
		EventType: eventOriginal, Event: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "texto",
		ExtractionTimeMS: 342, MimeType: "application/pdf",
		Metadata: domain.Metadata{Filename: "x.pdf"},
	})
	require.NoError(t, err)

	doc, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusPending, doc.Status)
	require.Equal(t, "chk-1", doc.Checksum)
	require.Equal(t, "texto", doc.ExtractedText)
	require.Equal(t, int64(342), doc.ExtractionTimeMS)
	require.Equal(t, "application/pdf", doc.Metadata.MimeType)
	require.Equal(t, "x.pdf", doc.Metadata.Filename)
}

func TestOnOriginal_DuplicateChecksum_Swallows(t *testing.T) {
	h, _ := newHandler()
	first := &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "a",
	}
	second := &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-2", Checksum: "chk-1", ExtractedText: "b",
	}

	require.NoError(t, h.OnOriginal(context.Background(), first))
	// Dup legítimo (cheque at-least-once) ⇒ nil, no error (ACK + descarte).
	require.NoError(t, h.OnOriginal(context.Background(), second))
}

func TestOnOriginal_DuplicateDocumentID_Swallows(t *testing.T) {
	h, _ := newHandler()
	ev := &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: "chk-1",
	}

	require.NoError(t, h.OnOriginal(context.Background(), ev))
	require.NoError(t, h.OnOriginal(context.Background(), ev))
}

func TestOnSummaryResolved_CompletesDocument(t *testing.T) {
	h, repo := newHandler()
	require.NoError(t, h.OnOriginal(context.Background(), &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "a",
	}))

	err := h.OnSummaryResolved(context.Background(), &SummaryResolvedEvent{
		EventType: eventSummaryResolved, SchemaVersion: 1,
		DocumentID: "doc-1", Summary: "resumen", SummaryTimeMS: 120,
	})
	require.NoError(t, err)

	doc, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, doc.Status)
	require.Equal(t, "resumen", *doc.Summary)
	require.Equal(t, int64(120), doc.SummaryTimeMS)
	require.True(t, doc.UpdatedAt.After(doc.CreatedAt))
}

func TestOnSummaryResolved_Orphan_ReturnsErrNotFound(t *testing.T) {
	h, _ := newHandler()

	err := h.OnSummaryResolved(context.Background(), &SummaryResolvedEvent{
		EventType: eventSummaryResolved, SchemaVersion: 1,
		DocumentID: "no-existe", Summary: "huérfano",
	})

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestOnSummaryResolved_AlreadyCompleted_IsBenign(t *testing.T) {
	h, repo := newHandler()
	require.NoError(t, h.OnOriginal(context.Background(), &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: "chk-1", ExtractedText: "a",
	}))
	require.NoError(t, h.OnSummaryResolved(context.Background(), &SummaryResolvedEvent{
		EventType: eventSummaryResolved, SchemaVersion: 1,
		DocumentID: "doc-1", Summary: "primera vez", SummaryTimeMS: 120,
	}))

	// Re-entrega at-least-once: nil ⇒ ACK + descarte, sin tocar el summary.
	require.NoError(t, h.OnSummaryResolved(context.Background(), &SummaryResolvedEvent{
		EventType: eventSummaryResolved, SchemaVersion: 1,
		DocumentID: "doc-1", Summary: "otra vez", SummaryTimeMS: 200,
	}))

	doc, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	require.Equal(t, "primera vez", *doc.Summary)
	require.Equal(t, int64(120), doc.SummaryTimeMS)
}
