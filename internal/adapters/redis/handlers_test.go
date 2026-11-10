package redis

import (
	"context"
	"fmt"
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

// seedOriginal ingesta un evento original y devuelve el documento persistido.
func seedOriginal(t *testing.T, h *MessageHandlers, repo *testutil.MemoryRepository, ev *OriginalEvent) domain.Document {
	t.Helper()
	require.NoError(t, h.OnOriginal(context.Background(), ev))
	doc, err := repo.FindByDocumentID(context.Background(), ev.DocumentID)
	require.NoError(t, err)
	return *doc
}

func TestOnOriginal_CreatesPendingDocument(t *testing.T) {
	h, repo := newHandler()

	doc := seedOriginal(t, h, repo, &OriginalEvent{
		EventType: eventOriginal, Event: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: "f46e3b17a6b639e5efddc3d1498ad73491599c22220f7ef6e14772f4ee25b913", ExtractedText: "texto",
		ExtractionTimeMS: 342, MimeType: "application/pdf",
		SummaryRequested: true,
		Filename:         "informe.pdf",
		Metadata:         domain.Metadata{Filename: "x.pdf"},
	})

	require.Equal(t, domain.SummaryPending, doc.SummaryStatus)
	require.Equal(t, "f46e3b17a6b639e5efddc3d1498ad73491599c22220f7ef6e14772f4ee25b913", doc.Checksum)
	require.Equal(t, "texto", doc.ExtractedText)
	require.Equal(t, int64(342), doc.ExtractionTimeMS)
	require.Equal(t, "application/pdf", doc.Metadata.MimeType)
	require.Equal(t, "informe.pdf", doc.Metadata.Filename)
}

func TestOnOriginal_NormalizesSummaryStatus(t *testing.T) {
	cases := []struct {
		name      string
		requested bool
		declared  string
		want      domain.SummaryStatus
	}{
		{"pedido, pendiente", true, "PENDING", domain.SummaryPending},
		{"pedido, resuelto en el original", true, "RESOLVED", domain.SummaryPending},
		{"pedido, sin estado declarado", true, "", domain.SummaryPending},
		{"no pedido", false, "NOT_REQUESTED", domain.SummaryNotRequested},
		{"fallido", true, "FAILED", domain.SummaryFailed},
	}

	checksumTexto := "f46e3b17a6b639e5efddc3d1498ad73491599c22220f7ef6e14772f4ee25b913"
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, repo := newHandler()
			id := fmt.Sprintf("doc-%d", i)
			doc := seedOriginal(t, h, repo, &OriginalEvent{
				EventType: eventOriginal, SchemaVersion: 1,
				DocumentID: id, Checksum: checksumTexto, ExtractedText: "texto",
				SummaryRequested: tc.requested, SummaryStatus: tc.declared,
			})

			require.Equal(t, tc.want, doc.SummaryStatus)
		})
	}
}

func TestOnOriginal_FilenameFallsBackToMetadata(t *testing.T) {
	h, repo := newHandler()

	doc := seedOriginal(t, h, repo, &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: "f46e3b17a6b639e5efddc3d1498ad73491599c22220f7ef6e14772f4ee25b913", ExtractedText: "texto",
		Metadata: domain.Metadata{Filename: "x.pdf"},
	})

	require.Equal(t, "x.pdf", doc.Metadata.Filename)
}

func TestOnOriginal_DuplicateChecksum_Swallows(t *testing.T) {
	h, _ := newHandler()
	checksumA := "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb" // sha256("a")
	checksumB := "3e23e8160039594a33894f6564e1b1348bbd7a0088d42c4acb73eeaed59c009d" // sha256("b")
	first := &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: checksumA, ExtractedText: "a",
	}
	second := &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-2", Checksum: checksumB, ExtractedText: "b",
	}

	require.NoError(t, h.OnOriginal(context.Background(), first))
	// Dup legítimo (cheque at-least-once) ⇒ nil, no error (ACK + descarte).
	require.NoError(t, h.OnOriginal(context.Background(), second))
}

func TestOnOriginal_DuplicateDocumentID_Swallows(t *testing.T) {
	h, _ := newHandler()
	checksum := "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb" // sha256("a")
	ev := &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: checksum, ExtractedText: "a",
	}

	require.NoError(t, h.OnOriginal(context.Background(), ev))
	require.NoError(t, h.OnOriginal(context.Background(), ev))
}

func TestOnSummaryResolved_CompletesDocument(t *testing.T) {
	h, repo := newHandler()
	checksumA := "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb" // sha256("a")
	seedOriginal(t, h, repo, &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: checksumA, ExtractedText: "a",
	})

	err := h.OnSummaryResolved(context.Background(), &SummaryResolvedEvent{
		EventType: eventSummaryResolved, SchemaVersion: 1,
		DocumentID: "doc-1", Summary: "resumen", SummaryTimeMS: 120,
	})
	require.NoError(t, err)

	doc, err := repo.FindByDocumentID(context.Background(), "doc-1")
	require.NoError(t, err)
	testutil.RequireSummaryResolved(t, *doc, "resumen", 120)
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
	checksumA := "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb" // sha256("a")
	seedOriginal(t, h, repo, &OriginalEvent{
		EventType: eventOriginal, SchemaVersion: 1,
		DocumentID: "doc-1", Checksum: checksumA, ExtractedText: "a",
	})
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

func TestOnOriginal_ChecksumMismatch_ReturnsError(t *testing.T) {
	h, _ := newHandler()
	// checksum "wrong" != sha256("texto")
	ev := &OriginalEvent{
		EventType:        eventOriginal,
		SchemaVersion:    1,
		DocumentID:       "doc-1",
		Checksum:         "wrong",
		ExtractedText:    "texto",
		ExtractionTimeMS: 342,
		MimeType:         "application/pdf",
	}

	err := h.OnOriginal(context.Background(), ev)

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalidRequest)
	require.Contains(t, err.Error(), "expected")
}

func TestOnSummaryResolved_SoftDeleted_ReturnsErrNotFound(t *testing.T) {
	h, repo := newHandler()
	// checksum must match sha256("a")
	seedOriginal(t, h, repo, &OriginalEvent{
		EventType:        eventOriginal,
		SchemaVersion:    1,
		DocumentID:       "doc-1",
		Checksum:         "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
		ExtractedText:    "a",
		ExtractionTimeMS: 100,
		MimeType:         "application/pdf",
	})
	// Soft-delete the document
	require.NoError(t, repo.SoftDelete(context.Background(), "doc-1"))

	err := h.OnSummaryResolved(context.Background(), &SummaryResolvedEvent{
		EventType: eventSummaryResolved, SchemaVersion: 1,
		DocumentID: "doc-1", Summary: "resumen", SummaryTimeMS: 120,
	})

	require.ErrorIs(t, err, domain.ErrNotFound)
}
