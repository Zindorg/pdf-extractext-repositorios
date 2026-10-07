package testutil

import (
	"context"
	"sort"
	"strings"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// MemoryRepository es un fake en-memoria del port domain.DocumentRepository
// con dedup por document_id/checksum y soft-delete que libera el checksum.
// Constructor único compartido por los tests unitarios (application) y los de
// integración (redis), para que ambos niveles prueben la misma semántica.
type MemoryRepository struct {
	byID       map[string]domain.Document // document_id → documento
	checksumID map[string]string          // checksum → document_id (activos)
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		byID:       make(map[string]domain.Document),
		checksumID: make(map[string]string),
	}
}

func (r *MemoryRepository) Insert(_ context.Context, doc domain.Document) error {
	if _, ok := r.byID[doc.DocumentID]; ok {
		return domain.ErrDuplicateDocumentID
	}
	if id, ok := r.checksumID[doc.Checksum]; ok && !r.byID[id].IsDeleted() {
		return domain.ErrDuplicateChecksum
	}
	r.byID[doc.DocumentID] = doc
	r.checksumID[doc.Checksum] = doc.DocumentID
	return nil
}

// UpdateStatus persiste el cambio de estado (fake fiel del port):
// inexistente → ErrNotFound (huérfano); existente → COMPLETED + updated_at.
func (r *MemoryRepository) UpdateStatus(_ context.Context, documentID string, status domain.Status, summary *string, summaryTimeMS int64) (*domain.Document, error) {
	doc, ok := r.byID[documentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	doc.Status = status
	doc.Summary = summary
	doc.SummaryTimeMS = summaryTimeMS
	doc.UpdatedAt = domain.Now()
	r.byID[documentID] = doc
	return &doc, nil
}

func (r *MemoryRepository) FindByDocumentID(_ context.Context, documentID string) (*domain.Document, error) {
	doc, ok := r.byID[documentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &doc, nil
}

func (r *MemoryRepository) FindByChecksum(ctx context.Context, checksum string) (*domain.Document, error) {
	id, ok := r.checksumID[checksum]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.FindByDocumentID(ctx, id)
}

// List asume page/pageSize ya normalizados por el servicio (ver puerto).
func (r *MemoryRepository) List(_ context.Context, filter domain.ListFilter, page, pageSize int) ([]domain.Document, int64, error) {
	filtered := r.matchFilter(filter)
	total := int64(len(filtered))

	// Pagination
	start := (page - 1) * pageSize
	if start >= len(filtered) {
		return []domain.Document{}, total, nil
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

// matchFilter devuelve los documentos que cumplen el filtro, ordenados por
// created_at DESC (más reciente primero).
func (r *MemoryRepository) matchFilter(filter domain.ListFilter) []domain.Document {
	var filtered []domain.Document
	for _, doc := range r.byID {
		if !matches(filter, doc) {
			continue
		}
		filtered = append(filtered, doc)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return filtered
}

func matches(filter domain.ListFilter, doc domain.Document) bool {
	if filter.Status != nil && doc.Status != *filter.Status {
		return false
	}
	if filter.Filename != nil {
		filename := strings.ToLower(doc.Metadata.Filename)
		search := strings.ToLower(*filter.Filename)
		if !strings.Contains(filename, search) {
			return false
		}
	}
	if filter.CreatedFrom != nil && doc.CreatedAt.Before(*filter.CreatedFrom) {
		return false
	}
	if filter.CreatedTo != nil && doc.CreatedAt.After(*filter.CreatedTo) {
		return false
	}
	return filter.IncludeDeleted || !doc.IsDeleted()
}

func (r *MemoryRepository) SoftDelete(_ context.Context, documentID string) error {
	doc, ok := r.byID[documentID]
	if !ok {
		return domain.ErrNotFound
	}
	now := domain.Now()
	doc.DeletedAt = &now
	doc.UpdatedAt = now
	r.byID[documentID] = doc
	delete(r.checksumID, doc.Checksum) // simula índice parcial: el checksum se libera
	return nil
}

func (r *MemoryRepository) Restore(_ context.Context, documentID string) error {
	doc, ok := r.byID[documentID]
	if !ok {
		return domain.ErrNotFound
	}
	if !doc.IsDeleted() {
		return nil // idempotente: ya activo
	}

	// Conflicto: ¿otro documento ACTIVO usa este checksum?
	if id, ok := r.checksumID[doc.Checksum]; ok && !r.byID[id].IsDeleted() {
		return domain.ErrRestoreConflict
	}

	doc.DeletedAt = nil
	doc.UpdatedAt = domain.Now()
	r.byID[documentID] = doc
	r.checksumID[doc.Checksum] = documentID // reapropia el checksum
	return nil
}
