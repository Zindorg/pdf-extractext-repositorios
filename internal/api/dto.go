package api

import (
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// deliveryAvailable es el único valor de delivery_status observable por esta
// API: el Persistidor solo expone documentos que ya asentó.
const deliveryAvailable = "AVAILABLE"

// DocumentResponse es el DTO de salida: expone la identidad de negocio,
// nunca el `_id` interno de Mongo. extracted_text y summary se omiten
// cuando include_text=false (omitempty).
type DocumentResponse struct {
	DocumentID       string          `json:"document_id"`
	Filename         string          `json:"filename"`
	MimeType         string          `json:"mime_type"`
	Checksum         string          `json:"checksum"`
	DeliveryStatus   string          `json:"delivery_status"`
	SummaryStatus    string          `json:"summary_status"`
	ExtractedText    string          `json:"extracted_text,omitempty"`
	Summary          *string         `json:"summary,omitempty"`
	Metadata         domain.Metadata `json:"metadata"`
	ExtractionTimeMS int64           `json:"extraction_time_ms"`
	SummaryTimeMS    *int64          `json:"summary_time_ms,omitempty"`
	ProcessingTimeMS int64           `json:"processing_time_ms"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	DeletedAt        *time.Time      `json:"deleted_at,omitempty"`
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

// ChecksumResponse es el DTO para GET /checksum/{checksum}: metadatos + summary.
// extracted_text se omite cuando include_text=false (omitempty).
type ChecksumResponse struct {
	DocumentID    string  `json:"document_id"`
	SummaryStatus string  `json:"summary_status"`
	Summary       *string `json:"summary,omitempty"`
	ExtractedText string  `json:"extracted_text,omitempty"`
}

func toResponse(doc domain.Document, includeText bool) DocumentResponse {
	resp := DocumentResponse{
		DocumentID:       doc.DocumentID,
		Filename:         doc.Metadata.Filename,
		MimeType:         doc.Metadata.MimeType,
		Checksum:         doc.Checksum,
		DeliveryStatus:   deliveryAvailable,
		SummaryStatus:    string(doc.SummaryStatus),
		Metadata:         doc.Metadata,
		ExtractionTimeMS: doc.ExtractionTimeMS,
		SummaryTimeMS:    summaryTimeMS(doc),
		ProcessingTimeMS: doc.ExtractionTimeMS + doc.SummaryTimeMS,
		CreatedAt:        doc.CreatedAt,
		UpdatedAt:        doc.UpdatedAt,
		DeletedAt:        doc.DeletedAt,
	}
	if includeText {
		resp.ExtractedText = doc.ExtractedText
		resp.Summary = doc.Summary
	}
	return resp
}

func summaryTimeMS(doc domain.Document) *int64 {
	if doc.Summary == nil {
		return nil
	}
	return &doc.SummaryTimeMS
}

func listResponse(items []domain.Document, total int64, page, pageSize int, includeText bool) ListResponse {
	resp := make([]DocumentResponse, len(items))
	for i, doc := range items {
		resp[i] = toResponse(doc, includeText)
	}
	return ListResponse{Items: resp, Total: total, Page: page, PageSize: pageSize}
}
