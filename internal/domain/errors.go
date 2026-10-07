package domain

import "errors"

var (
	ErrNotFound            = errors.New("document not found")
	ErrDuplicateChecksum   = errors.New("duplicate checksum")
	ErrDuplicateDocumentID = errors.New("duplicate document id")
	ErrSummaryPending      = errors.New("summary not ready")
	ErrRestoreConflict     = errors.New("restore conflict: checksum already in use")
)
