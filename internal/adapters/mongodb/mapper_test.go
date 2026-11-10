package mongodb

import (
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPersistedRoundTrip(t *testing.T) {
	doc := testutil.CompleteDoc()
	deletedAt := time.Now().UTC()
	doc.DeletedAt = &deletedAt

	got := toDomain(toPersisted(doc))

	testutil.RequireSameContent(t, doc, got)
	require.Equal(t, doc.Summary, got.Summary)
	require.Equal(t, doc.Metadata, got.Metadata)
	require.Equal(t, doc.ExtractionTimeMS, got.ExtractionTimeMS)
	require.Equal(t, doc.SummaryTimeMS, got.SummaryTimeMS)
	require.Equal(t, doc.CreatedAt, got.CreatedAt)
	require.Equal(t, doc.UpdatedAt, got.UpdatedAt)
	require.Equal(t, doc.DeletedAt, got.DeletedAt)
}

func TestPersistedRoundTrip_NoSummaryNoDeletedAt(t *testing.T) {
	got := toDomain(toPersisted(domain.Document{DocumentID: "doc-2"}))

	require.Equal(t, "doc-2", got.DocumentID)
	require.Nil(t, got.Summary)
	require.Nil(t, got.DeletedAt)
	require.Equal(t, domain.SummaryStatus(""), got.SummaryStatus)
}
