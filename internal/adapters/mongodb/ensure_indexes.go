package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ensureIndexes crea los índices definidos en ARCHITECTURE.md §8:
//   - uq_document_id          : único no parcial
//   - uq_checksum_active      : único parcial (deleted_at == null) → dedup de activos
//   - ix_filename             : búsqueda por nombre de archivo
//   - ix_created_at           : ordenamiento/consulta por fecha de creación
func ensureIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "document_id", Value: 1}},
			Options: options.Index().
				SetName("uq_document_id").
				SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "checksum", Value: 1}},
			Options: options.Index().
				SetName("uq_checksum_active").
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys:    bson.D{{Key: "metadata.filename", Value: 1}},
			Options: options.Index().SetName("ix_filename"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("ix_created_at"),
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	return err
}

// EnsureIndexes expone la creación de índices al composition root.
func (r *MongoDocumentRepository) EnsureIndexes(ctx context.Context) error {
	return ensureIndexes(ctx, r.collection)
}
