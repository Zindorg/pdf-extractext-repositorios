package mongodb

import (
	"context"
	"errors"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionName = "documents"

// MongoDocumentRepository implementa el puerto del dominio sobre MongoDB.
// Insert, Complete, Find*, List, SoftDelete y Restore están implementados.
type MongoDocumentRepository struct {
	collection *mongo.Collection
}

func NewMongoDocumentRepository(db *mongo.Database) *MongoDocumentRepository {
	return &MongoDocumentRepository{
		collection: db.Collection(collectionName),
	}
}

func (r *MongoDocumentRepository) Insert(ctx context.Context, doc domain.Document) error {
	persisted := toPersisted(doc)
	persisted.ID = bson.NewObjectID() // el driver no genera _id si el campo viene en cero
	if _, err := r.collection.InsertOne(ctx, persisted); err != nil {
		return mapInsertError(err)
	}
	return nil
}

func (r *MongoDocumentRepository) Complete(ctx context.Context, documentID string, summary string, summaryTimeMS int64) (*domain.Document, error) {
	filter := bson.M{"document_id": documentID}
	update := bson.M{"$set": bson.M{
		"summary_status":  string(domain.SummaryResolved),
		"summary":         summary,
		"summary_time_ms": summaryTimeMS,
		"updated_at":      domain.Now(),
	}}

	res := r.collection.FindOneAndUpdate(ctx, filter, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After))
	return decodeSingle(res)
}

// findOne ejecuta FindOne con filtro genérico y devuelve *domain.Document.
func (r *MongoDocumentRepository) findOne(ctx context.Context, filter any) (*domain.Document, error) {
	return decodeSingle(r.collection.FindOne(ctx, filter))
}

// decodeSingle traduce ErrNoDocuments a ErrNotFound y decodifica el
// resultado (compartido por FindOne y FindOneAndUpdate).
func decodeSingle(res *mongo.SingleResult) (*domain.Document, error) {
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	var persisted persistedDocument
	if err := res.Decode(&persisted); err != nil {
		return nil, err
	}
	doc := toDomain(persisted)
	return &doc, nil
}

func (r *MongoDocumentRepository) FindByDocumentID(ctx context.Context, documentID string) (*domain.Document, error) {
	return r.findOne(ctx, bson.M{"document_id": documentID})
}

func (r *MongoDocumentRepository) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	return r.findOne(ctx, bson.M{"checksum": checksum})
}

func (r *MongoDocumentRepository) List(ctx context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	return r.findPage(ctx, buildMongoFilter(filter), page, pageSize)
}
