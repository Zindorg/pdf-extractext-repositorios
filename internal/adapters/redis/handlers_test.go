package redis

import (
	"context"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

// handlerRepo es un fake del puerto DocumentRepository para probar los
// handlers con un DocumentService real (seam del consumer).
type handlerRepo struct {
	byID       map[string]domain.Document // document_id → documento
	checksumID map[string]string          // checksum → document_id (activos)
}

func newHandlerRepo() *handlerRepo {
	return &handlerRepo{
		byID:       make(map[string]domain.Document),
		checksumID: make(map[string]string),
	}
}

func (r *handlerRepo) Insert(_ context.Context, doc domain.Document) error {
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

func (r *handlerRepo) UpdateStatus(_ context.Context, documentID string, status domain.Status, summary *string, summaryTimeMS int64) (*domain.Document, error) {
	doc, ok := r.byID[documentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	doc.Status = status
	doc.Summary = summary
	doc.SummaryTimeMS = summaryTimeMS
	doc.UpdatedAt = domain.Now()
	r.byID[documentID] = doc
	return &doc, nil
}

func (r *handlerRepo) FindByDocumentID(_ context.Context, documentID string) (*domain.Document, error) {
	doc, ok := r.byID[documentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &doc, nil
}

func (r *handlerRepo) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	id, ok := r.checksumID[checksum]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.FindByDocumentID(ctx, id)
}

func (r *handlerRepo) List(context.Context, domain.ListFilter, int, int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}

func (r *handlerRepo) SoftDelete(context.Context, string) error { return nil }
func (r *handlerRepo) Restore(context.Context, string) error    { return nil }

func newHandler() (*MessageHandlers, *handlerRepo) {
	repo := newHandlerRepo()
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
