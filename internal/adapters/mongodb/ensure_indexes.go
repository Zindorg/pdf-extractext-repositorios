package mongodb

import (
	"context"
	"errors"
	"fmt"

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
	idxSummaryStatus = "ix_summary_status"
)

// Códigos de error de MongoDB al borrar un índice que no existe.
const (
	errCodeNamespaceNotFound = 26
	errCodeIndexNotFound     = 27
)

// EnsureIndexes crea los índices definidos en ARCHITECTURE.md §8.
func (r *MongoDocumentRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, indexModels())
	return err
}

// Reindex borra y recrea los índices a partir de la MISMA definición que EnsureIndexes
// (12 Factor XII: el proceso administrativo comparte código con la app, sin listas duplicadas).
//
// CUIDADO: entre el borrado y la recreación no existen los índices únicos. Ejecutarlo con el
// consumer detenido (los eventos esperan en el stream); si hay duplicados, la recreación falla.
func (r *MongoDocumentRepository) Reindex(ctx context.Context) error {
	for _, s := range indexSpecs() {
		if err := r.collection.Indexes().DropOne(ctx, s.name); err != nil && !isIndexMissing(err) {
			return fmt.Errorf("drop index %s: %w", s.name, err)
		}
	}
	if err := r.EnsureIndexes(ctx); err != nil {
		return fmt.Errorf("recreate indexes: %w", err)
	}
	return nil
}

// isIndexMissing indica que el índice (o la colección) no existe: borrarlo no es un error.
func isIndexMissing(err error) bool {
	var ce mongo.CommandError
	return errors.As(err, &ce) && (ce.Code == errCodeIndexNotFound || ce.Code == errCodeNamespaceNotFound)
}

// indexSpec une el nombre de un índice con su modelo: así el nombre y la definición
// no pueden desincronizarse entre EnsureIndexes y Reindex.
type indexSpec struct {
	name  string
	model mongo.IndexModel
}

func newIndexSpec(name string, keys bson.D, opts *options.IndexOptionsBuilder) indexSpec {
	return indexSpec{name: name, model: mongo.IndexModel{Keys: keys, Options: opts.SetName(name)}}
}

// indexSpecs define los índices:
//   - uq_document_id     : único no parcial
//   - uq_checksum_active : único parcial (deleted_at == null) → dedup de activos
//   - ix_filename        : búsqueda por nombre de archivo
//   - ix_created_at      : ordenamiento/consulta por fecha de creación
//   - ix_summary_status  : consulta por estado del resumen
func indexSpecs() []indexSpec {
	return []indexSpec{
		newIndexSpec(idxDocumentID,
			bson.D{{Key: "document_id", Value: 1}},
			options.Index().SetUnique(true)),
		newIndexSpec(idxChecksumAlive,
			bson.D{{Key: "checksum", Value: 1}},
			options.Index().SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}})),
		newIndexSpec(idxFilename,
			bson.D{{Key: "metadata.filename", Value: 1}},
			options.Index()),
		newIndexSpec(idxCreatedAt,
			bson.D{{Key: "created_at", Value: -1}},
			options.Index()),
		newIndexSpec(idxSummaryStatus,
			bson.D{{Key: "summary_status", Value: 1}},
			options.Index()),
	}
}

// indexModels devuelve los modelos de índice para CreateMany.
func indexModels() []mongo.IndexModel {
	specs := indexSpecs()
	models := make([]mongo.IndexModel, len(specs))
	for i, s := range specs {
		models[i] = s.model
	}
	return models
}