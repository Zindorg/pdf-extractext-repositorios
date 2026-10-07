package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Nombres de índice: una sola fuente; la clasificación de errores 11000 en
// repository.go compara contra estos mismos valores.
const (
	idxDocumentID    = "uq_document_id"
	idxChecksumAlive = "uq_checksum_active"
	idxFilename      = "ix_filename"
	idxCreatedAt     = "ix_created_at"
)

// EnsureIndexes crea los índices definidos en ARCHITECTURE.md §8.
func (r *MongoDocumentRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, indexModels())
	return err
}

// indexModels define los índices:
//   - uq_document_id     : único no parcial
//   - uq_checksum_active : único parcial (deleted_at == null) → dedup de activos
//   - ix_filename        : búsqueda por nombre de archivo
//   - ix_created_at      : ordenamiento/consulta por fecha de creación
func indexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "document_id", Value: 1}},
			Options: options.Index().SetName(idxDocumentID).SetUnique(true)},
		{Keys: bson.D{{Key: "checksum", Value: 1}},
			Options: options.Index().SetName(idxChecksumAlive).SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}})},
		{Keys: bson.D{{Key: "metadata.filename", Value: 1}},
			Options: options.Index().SetName(idxFilename)},
		{Keys: bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName(idxCreatedAt)},
	}
}
