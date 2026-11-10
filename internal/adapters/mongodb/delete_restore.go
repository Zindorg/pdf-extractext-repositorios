package mongodb

import (
	"context"
	"errors"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

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
