package mongodb

import (
	"errors"
	"strings"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

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

var (
	errDuplicate = errors.New("duplicate key")
)
