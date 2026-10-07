package mongodb

import (
	"context"
	"errors"
	"strings"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionName = "documents"

// MongoDocumentRepository implementa el puerto del dominio sobre MongoDB.
// Insert, UpdateStatus, Find*, List, SoftDelete y Restore están implementados.
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

// mapInsertError traduce un error de escritura de Mongo a los errores del
// dominio. La garantía fuerte de dedup vive en los índices únicos
// (uq_document_id y uq_checksum_active); acá solo se clasifica el 11000.
func mapInsertError(err error) error {
	if !mongo.IsDuplicateKeyError(err) {
		return err
	}
	switch {
	case strings.Contains(err.Error(), idxDocumentID):
		return domain.ErrDuplicateDocumentID
	case strings.Contains(err.Error(), idxChecksumAlive):
		return domain.ErrDuplicateChecksum
	default:
		return errDuplicate
	}
}

func (r *MongoDocumentRepository) UpdateStatus(ctx context.Context, documentID string, status domain.Status, summary *string, summaryTimeMS int64) (*domain.Document, error) {
	filter := bson.M{"document_id": documentID}
	update := bson.M{"$set": bson.M{
		"status":          string(status),
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
	if f.Status != nil {
		m["status"] = *f.Status
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

func (r *MongoDocumentRepository) SoftDelete(ctx context.Context, documentID string) error {
	filter := bson.M{"document_id": documentID}
	update := bson.M{"$set": bson.M{
		"deleted_at": domain.Now(),
		"updated_at": domain.Now(),
	}}

	res, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// restoreConflictCheck verifica si existe un documento ACTIVO con el mismo
// checksum (excluyendo el propio documentID). ErrRestoreConflict si colisiona.
func (r *MongoDocumentRepository) restoreConflictCheck(ctx context.Context, documentID, checksum string) error {
	_, err := r.findOne(ctx, bson.M{
		"checksum":    checksum,
		"document_id": bson.M{"$ne": documentID},
		"deleted_at":  bson.M{"$exists": false},
	})
	if err == nil {
		return domain.ErrRestoreConflict
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

// clearDeletedAt limpia el campo deleted_at (restaura) con su marca en updated_at.
func (r *MongoDocumentRepository) clearDeletedAt(ctx context.Context, documentID string) error {
	filter := bson.M{"document_id": documentID}
	update := bson.M{
		"$unset": bson.M{"deleted_at": ""},
		"$set":   bson.M{"updated_at": domain.Now()},
	}
	res, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MongoDocumentRepository) Restore(ctx context.Context, documentID string) error {
	doc, err := r.findOne(ctx, bson.M{"document_id": documentID})
	if err != nil {
		return err
	}
	if !doc.IsDeleted() {
		return nil // idempotente: ya activo
	}
	if err := r.restoreConflictCheck(ctx, documentID, doc.Checksum); err != nil {
		return err
	}
	return r.clearDeletedAt(ctx, documentID)
}

var (
	errNotImplemented = errors.New("not implemented yet")
	errDuplicate      = errors.New("duplicate key")
)
