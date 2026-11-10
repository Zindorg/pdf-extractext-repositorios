package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestList_DefaultPagination(t *testing.T) {
	svc, repo := newService()

	seedListDocs(t, repo, 25)

	docs, total, err := svc.List(context.Background(), domain.ListFilter{}, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(25), total)
	require.Len(t, docs, 20) // default pageSize=20
	// Verifica orden: created_at DESC (más reciente primero)
	require.True(t, docs[0].CreatedAt.After(docs[1].CreatedAt))
}

func TestList_CustomPagination(t *testing.T) {
	svc, repo := newService()

	seedListDocs(t, repo, 25)

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
	svc, repo := newService()

	testutil.SeedDoc(t, repo, makeListDoc("doc-1", "PENDING"))
	testutil.SeedDoc(t, repo, makeListDoc("doc-2", "RESOLVED"))
	testutil.SeedDoc(t, repo, makeListDoc("doc-3", "PENDING"))

	pending := domain.SummaryPending
	docs, total := testutil.ListDocs(t, svc, domain.ListFilter{SummaryStatus: &pending})

	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
	testutil.RequireAllStatus(t, docs, domain.SummaryPending)
}

func TestList_FilterByFilename(t *testing.T) {
	svc, repo := newService()

	testutil.SeedDoc(t, repo, testutil.NewListDoc("doc-1", "informe.pdf", domain.SummaryPending, time.Time{}))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("doc-2", "factura.pdf", domain.SummaryPending, time.Time{}))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("doc-3", "INFORME_anual.pdf", domain.SummaryPending, time.Time{}))

	fn := "informe"
	_, total := testutil.ListDocs(t, svc, domain.ListFilter{Filename: &fn})

	require.Equal(t, int64(2), total) // "informe.pdf" + "INFORME_anual.pdf" (case-insensitive)
}

func TestList_FilterByCreatedRange(t *testing.T) {
	svc, repo := newService()

	now := time.Now().UTC()
	testutil.SeedDoc(t, repo, testutil.NewListDoc("old", "", domain.SummaryPending, now.Add(-48*time.Hour)))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("mid", "", domain.SummaryPending, now.Add(-24*time.Hour)))
	testutil.SeedDoc(t, repo, testutil.NewListDoc("new", "", domain.SummaryPending, now.Add(-1*time.Hour)))

	from := now.Add(-36 * time.Hour)
	to := now.Add(-12 * time.Hour)
	docs, total := testutil.ListDocs(t, svc, domain.ListFilter{CreatedFrom: &from, CreatedTo: &to})

	require.Equal(t, int64(1), total) // solo "mid"
	require.Equal(t, "mid", docs[0].DocumentID)
}

func TestList_IncludeDeleted(t *testing.T) {
	svc, repo := newService()

	testutil.SeedDoc(t, repo, makeListDoc("active", "PENDING"))
	insertSoftDeleted(t, repo, makeListDoc("deleted", "PENDING"))

	// Sin IncludeDeleted → excluye borrados
	docs, _ := testutil.ListDocs(t, svc, domain.ListFilter{})
	require.Len(t, docs, 1)
	require.Equal(t, "active", docs[0].DocumentID)

	// Con IncludeDeleted=true → incluye borrados
	docs, total := testutil.ListDocs(t, svc, domain.ListFilter{IncludeDeleted: true})
	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
}

func TestList_ExcludesDeletedByDefault(t *testing.T) {
	svc, repo := newService()

	insertSoftDeleted(t, repo, makeListDoc("deleted", "PENDING"))

	_, total := testutil.ListDocs(t, svc, domain.ListFilter{})
	require.Equal(t, int64(0), total)
}

func TestList_ReturnsTotal(t *testing.T) {
	svc, repo := newService()

	seedListDocs(t, repo, 42)

	_, total, err := svc.List(context.Background(), domain.ListFilter{}, 2, 10)
	require.NoError(t, err)
	require.Equal(t, int64(42), total) // total correcto para paginación
}
