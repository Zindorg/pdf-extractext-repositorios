package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
)

// SeedDoc inserta doc en el repo y falla el test si el puerto la rechaza.
func SeedDoc(t *testing.T, repo domain.DocumentRepository, doc domain.Document) domain.Document {
	t.Helper()
	require.NoError(t, repo.Insert(context.Background(), doc))
	return doc
}

// SeedSoftDeleted inserta doc ya marcada como borrado lógico.
func SeedSoftDeleted(t *testing.T, repo domain.DocumentRepository, doc domain.Document, deletedAt time.Time) domain.Document {
	t.Helper()
	return SeedDoc(t, repo, WithDeleted(doc, deletedAt))
}
