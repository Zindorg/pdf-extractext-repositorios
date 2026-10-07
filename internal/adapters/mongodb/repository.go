package mongodb

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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
	if r.collection == nil {
		return errNotConnected
	}

	persisted := toPersisted(doc)
	persisted.ID = bson.NewObjectID() // el driver no genera _id si el campo viene en cero
	if _, err := r.collection.InsertOne(ctx, persisted); err != nil {
		return mapInsertError(err)
	}
	return nil
}

// mapInsertError traduce un error de escritura de Mongo a los errores del
// dominio. La garantía fuerte de dedup vive en los índices únicos
// (uq_document_id y uq_checksum_active); acá solo se clasifica el 11000.
func mapInsertError(err error) error {
	if !mongo.IsDuplicateKeyError(err) {
		return err
	}
	switch {
	case strings.Contains(err.Error(), "uq_document_id"):
		return domain.ErrDuplicateDocumentID
	case strings.Contains(err.Error(), "uq_checksum_active"):
		return domain.ErrDuplicateChecksum
	default:
		return errDuplicate
	}
}

func (r *MongoDocumentRepository) UpdateStatus(ctx context.Context, documentID string, status domain.Status, summary *string) (*domain.Document, error) {
	if r.collection == nil {
		return nil, errNotConnected
	}

	filter := bson.M{"document_id": documentID}
	update := bson.M{"$set": bson.M{
		"status":     string(status),
		"summary":    summary,
		"updated_at": time.Now().UTC(),
	}}

	res := r.collection.FindOneAndUpdate(ctx, filter, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After))
	if res.Err() != nil {
		if errors.Is(res.Err(), mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, res.Err()
	}

	var persisted persistedDocument
	if err := res.Decode(&persisted); err != nil {
		return nil, err
	}
	doc := toDomain(persisted)
	return &doc, nil
}

// findOne ejecuta FindOne con filtro genérico, mapea error y devuelve *domain.Document
func (r *MongoDocumentRepository) findOne(ctx context.Context, filter any) (*domain.Document, error) {
	if r.collection == nil {
		return nil, errNotConnected
	}
	res := r.collection.FindOne(ctx, filter)
	if res.Err() != nil {
		if errors.Is(res.Err(), mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, res.Err()
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
	if r.collection == nil {
		return nil, 0, errNotConnected
	}

	mongoFilter := buildMongoFilter(filter)

	total, err := r.collection.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize)).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := r.collection.Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var docs []domain.Document
	for cursor.Next(ctx) {
		var p persistedDocument
		if err := cursor.Decode(&p); err != nil {
			return nil, 0, err
		}
		docs = append(docs, toDomain(p))
	}
	return docs, total, nil
}

func buildMongoFilter(f domain.ListFilter) bson.M {
	m := bson.M{}
	if !f.IncludeDeleted {
		m["deleted_at"] = bson.M{"$exists": false}
	}
	if f.Status != nil {
		m["status"] = *f.Status
	}
	if f.Filename != nil {
		m["metadata.filename"] = bson.M{
			"$regex":   *f.Filename,
			"$options": "i",
		}
	}
	return m
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
