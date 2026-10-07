package mongodb

import (
	"errors"
	"testing"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func duplicateKeyError(message string) error {
	return &mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 11000, Message: message}}}
}

func TestMapInsertError_DuplicateDocumentID(t *testing.T) {
	err := mapInsertError(duplicateKeyError("E11000 duplicate key ... index: uq_document_id_1 dup key"))

	require.Equal(t, domain.ErrDuplicateDocumentID, err)
}

func TestMapInsertError_DuplicateChecksum(t *testing.T) {
	err := mapInsertError(duplicateKeyError("E11000 duplicate key ... index: uq_checksum_active_1 dup key"))

	require.Equal(t, domain.ErrDuplicateChecksum, err)
}

func TestMapInsertError_UnknownDuplicate(t *testing.T) {
	err := mapInsertError(duplicateKeyError("E11000 duplicate key ... index: otro_1 dup key"))

	require.Equal(t, errDuplicate, err)
}

func TestMapInsertError_NonDuplicatePassthrough(t *testing.T) {
	original := errors.New("timeout")

	require.Equal(t, original, mapInsertError(original))
}
