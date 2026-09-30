package mongodb

import (
	"context"
	"errors"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const collectionName = "documents"

// MongoDocumentRepository implementa el puerto del dominio sobre MongoDB.
// En esta fase es un esqueleto: la conexión y el mapper están listos; la
// lógica de cada operación se completa en las fases de lógica.
type MongoDocumentRepository struct {
	collection *mongo.Collection
}

func NewMongoDocumentRepository(db *mongo.Database) *MongoDocumentRepository {
	return &MongoDocumentRepository{
		collection: db.Collection(collectionName),
	}
}

func (r *MongoDocumentRepository) Insert(ctx context.Context, doc domain.Document) error {
	_ = doc
	if r.collection == nil {
		return errNotConnected
	}
	return errNotImplemented
}

func (r *MongoDocumentRepository) UpdateStatus(ctx context.Context, documentID string, status domain.Status, summary *string) (*domain.Document, error) {
	return nil, errNotImplemented
}

func (r *MongoDocumentRepository) FindByDocumentID(ctx context.Context, documentID string) (*domain.Document, error) {
	if r.collection == nil {
		return nil, errNotConnected
	}
	return nil, errNotImplemented
}

func (r *MongoDocumentRepository) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	if r.collection == nil {
		return nil, errNotConnected
	}
	return nil, errNotImplemented
}

func (r *MongoDocumentRepository) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	return nil, 0, errNotImplemented
}

func (r *MongoDocumentRepository) SoftDelete(ctx context.Context, documentID string) error {
	return errNotImplemented
}

func (r *MongoDocumentRepository) Restore(ctx context.Context, documentID string) error {
	return errNotImplemented
}

var (
	errNotImplemented = errors.New("not implemented yet")
	errNotConnected   = errors.New("mongodb not connected")
	errDuplicate      = errors.New("duplicate key")
)
