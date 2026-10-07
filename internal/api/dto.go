package api

import (
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// DocumentResponse es el DTO de salida: expone la identidad de negocio,
// nunca el `_id` interno de Mongo.
type DocumentResponse struct {
	DocumentID       string          `json:"document_id"`
	Checksum         string          `json:"checksum"`
	Status           string          `json:"status"`
	ExtractedText    string          `json:"extracted_text"`
	Summary          *string         `json:"summary"`
	Metadata         domain.Metadata `json:"metadata"`
	ProcessingTimeMS int64           `json:"processing_time_ms"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type ListResponse struct {
	Items    []DocumentResponse `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

func toResponse(doc domain.Document) DocumentResponse {
	return DocumentResponse{
		DocumentID:       doc.DocumentID,
		Checksum:         doc.Checksum,
		Status:           string(doc.Status),
		ExtractedText:    doc.ExtractedText,
		Summary:          doc.Summary,
		Metadata:         doc.Metadata,
		ProcessingTimeMS: doc.ExtractionTimeMS + doc.SummaryTimeMS,
		CreatedAt:        doc.CreatedAt,
		UpdatedAt:        doc.UpdatedAt,
	}
}
