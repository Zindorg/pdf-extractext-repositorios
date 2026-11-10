package mongodb

import (
	"context"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// findPage cuenta y pagina los documentos que cumplen filter (created_at DESC).
func (r *MongoDocumentRepository) findPage(ctx context.Context, filter bson.M, page, pageSize int) ([]domain.Document, int64, error) {
	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	cursor, err := r.collection.Find(ctx, filter, pageOptions(page, pageSize))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	docs, err := decodeCursor(ctx, cursor)
	if err != nil {
		return nil, 0, err
	}
	return docs, total, nil
}

// pageOptions arma skip/limit/sort del listado.
func pageOptions(page, pageSize int) *options.FindOptionsBuilder {
	return options.Find().
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize)).
		SetSort(bson.D{{Key: "created_at", Value: -1}})
}

// decodeCursor recorre el cursor y traduce a documentos de dominio;
// cualquier error del cursor aborta el listado.
func decodeCursor(ctx context.Context, cursor *mongo.Cursor) ([]domain.Document, error) {
	var docs []domain.Document
	for cursor.Next(ctx) {
		var p persistedDocument
		if err := cursor.Decode(&p); err != nil {
			return nil, err
		}
		docs = append(docs, toDomain(p))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return docs, nil
}

func buildMongoFilter(f domain.ListFilter) bson.M {
	m := bson.M{}
	if !f.IncludeDeleted {
		m["deleted_at"] = bson.M{"$exists": false}
	}
	if f.SummaryStatus != nil {
		m["summary_status"] = *f.SummaryStatus
	}
	if f.Filename != nil {
		m["metadata.filename"] = bson.M{
			"$regex":   *f.Filename,
			"$options": "i",
		}
	}
	applyCreatedRange(m, f)
	return m
}

// applyCreatedRange añade el rango $gte/$lte de created_at si el request lo trae.
func applyCreatedRange(m bson.M, f domain.ListFilter) {
	if f.CreatedFrom == nil && f.CreatedTo == nil {
		return
	}
	rangeFilter := bson.M{}
	if f.CreatedFrom != nil {
		rangeFilter["$gte"] = *f.CreatedFrom
	}
	if f.CreatedTo != nil {
		rangeFilter["$lte"] = *f.CreatedTo
	}
	m["created_at"] = rangeFilter
}
