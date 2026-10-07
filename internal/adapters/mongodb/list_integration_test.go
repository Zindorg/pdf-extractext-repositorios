//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/marco/pdf-extractext-repositorios/internal/testutil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// integrationURI devuelve la URI de Mongo para tests de integración.
// Se lee de MONGODB_URI; si no llega a conectar, el test se salta (skip)
// para no romper `make test` en entornos sin infraestructura.
func integrationURI() (string, bool) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return "", false
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return "", false
	}
	_ = client.Disconnect(context.Background())
	return uri, true
}

// newIntegrationRepo crea el repo sobre una base de test dedicada y la limpia.
func newIntegrationRepo(t *testing.T) *MongoDocumentRepository {
	t.Helper()
	uri, ok := integrationURI()
	if !ok {
		t.Skip("MongoDB no disponible para tests de integración")
		return nil
	}
	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(ctx) })

	dbName := "persister_test"
	db := client.Database(dbName)
	require.NoError(t, db.Drop(ctx))

	repo := NewMongoDocumentRepository(db)
	return repo
}

// insertDoc inserta un documento directo al repo (mirror de CreatePending).
func insertDoc(t *testing.T, repo *MongoDocumentRepository, doc domain.Document) {
	t.Helper()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	if doc.Status == "" {
		doc.Status = domain.StatusPending
	}
	require.NoError(t, repo.Insert(context.Background(), doc))
}

// integrationDoc envuelve el fixture compartido (testutil.NewDoc) con las
// convenciones de este test: checksum derivado del id.
func integrationDoc(id, filename, status string, created time.Time) domain.Document {
	return testutil.NewDoc(id, filename, "chk-"+id, domain.Status(status), created)
}

func TestList_Integration_DefaultPaginationExcludesDeleted(t *testing.T) {
	repo := newIntegrationRepo(t)

	now := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("doc-1", "a.pdf", "PENDING", now.Add(-3*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-2", "b.pdf", "COMPLETED", now.Add(-2*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-3", "c.pdf", "PENDING", now.Add(-1*time.Hour)))

	deletedAt := now.Add(-30 * time.Minute)
	del := testutil.WithDeleted(integrationDoc("doc-del", "d.pdf", "PENDING", now.Add(-30*time.Minute)), deletedAt)
	insertDoc(t, repo, del)

	docs, total, err := repo.List(context.Background(), domain.ListFilter{}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(3), total) // excluye soft-deleted
	require.Len(t, docs, 3)
	require.Equal(t, "doc-3", docs[0].DocumentID) // created_at DESC
	require.Equal(t, "doc-1", docs[2].DocumentID)
}

func TestList_Integration_FilterByStatus(t *testing.T) {
	repo := newIntegrationRepo(t)

	now := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("doc-1", "a.pdf", "PENDING", now.Add(-3*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-2", "b.pdf", "COMPLETED", now.Add(-2*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-3", "c.pdf", "PENDING", now.Add(-1*time.Hour)))

	pending := domain.StatusPending
	docs, total, err := repo.List(context.Background(), domain.ListFilter{Status: &pending}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
	for _, d := range docs {
		require.Equal(t, domain.StatusPending, d.Status)
	}
}

func TestList_Integration_FilterByFilename(t *testing.T) {
	repo := newIntegrationRepo(t)

	now := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("doc-1", "informe.pdf", "PENDING", now.Add(-3*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-2", "factura.pdf", "PENDING", now.Add(-2*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-3", "INFORME_anual.pdf", "PENDING", now.Add(-1*time.Hour)))

	fn := "informe"
	_, total, err := repo.List(context.Background(), domain.ListFilter{Filename: &fn}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total) // informe.pdf + INFORME_anual.pdf (case-insensitive)
}

func TestList_Integration_FilterByCreatedRange(t *testing.T) {
	repo := newIntegrationRepo(t)

	now := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("old", "a.pdf", "PENDING", now.Add(-48*time.Hour)))
	insertDoc(t, repo, integrationDoc("mid", "b.pdf", "PENDING", now.Add(-24*time.Hour)))
	insertDoc(t, repo, integrationDoc("new", "c.pdf", "PENDING", now.Add(-1*time.Hour)))

	from := now.Add(-36 * time.Hour)
	to := now.Add(-12 * time.Hour)
	docs, total, err := repo.List(context.Background(), domain.ListFilter{CreatedFrom: &from, CreatedTo: &to}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "mid", docs[0].DocumentID)
}

func TestList_Integration_IncludeDeleted(t *testing.T) {
	repo := newIntegrationRepo(t)

	now := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("active", "a.pdf", "PENDING", now.Add(-2*time.Hour)))
	deletedAt := now.Add(-time.Hour)
	del := testutil.WithDeleted(integrationDoc("deleted", "b.pdf", "PENDING", now.Add(-1*time.Hour)), deletedAt)
	insertDoc(t, repo, del)

	docs, _, err := repo.List(context.Background(), domain.ListFilter{}, 1, 20)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "active", docs[0].DocumentID)

	docs, total, err := repo.List(context.Background(), domain.ListFilter{IncludeDeleted: true}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, docs, 2)
}

func TestList_Integration_PaginationAndTotal(t *testing.T) {
	repo := newIntegrationRepo(t)

	now := time.Now().UTC()
	for i := 1; i <= 25; i++ {
		insertDoc(t, repo, integrationDoc(
			fmt.Sprintf("doc-%d", i), fmt.Sprintf("file-%d.pdf", i), "PENDING", now.Add(time.Duration(-i)*time.Hour),
		))
	}

	docs, total, err := repo.List(context.Background(), domain.ListFilter{}, 2, 10)
	require.NoError(t, err)
	require.Equal(t, int64(25), total)
	require.Len(t, docs, 10)
	// página 2 (items 11-20) ordenados DESC por created_at: doc-11 a doc-20
	require.Equal(t, "doc-11", docs[0].DocumentID)
	require.Equal(t, "doc-20", docs[9].DocumentID)
}

func TestList_Integration_CombinedFilters(t *testing.T) {
	repo := newIntegrationRepo(t)

	now := time.Now().UTC()
	insertDoc(t, repo, integrationDoc("doc-1", "informe.pdf", "PENDING", now.Add(-48*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-2", "informe.pdf", "COMPLETED", now.Add(-24*time.Hour)))
	insertDoc(t, repo, integrationDoc("doc-3", "factura.pdf", "PENDING", now.Add(-12*time.Hour)))

	pending := domain.StatusPending
	from := now.Add(-36 * time.Hour)
	to := now.Add(-6 * time.Hour)
	fn := "informe"

	docs, total, err := repo.List(context.Background(), domain.ListFilter{
		Status:      &pending,
		Filename:    &fn,
		CreatedFrom: &from,
		CreatedTo:   &to,
	}, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(0), total) // doc-1 fuera de rango, doc-2 completed, doc-3 otro filename
	require.Len(t, docs, 0)
}
