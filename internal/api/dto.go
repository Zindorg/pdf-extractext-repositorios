package api

import (
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// deliveryAvailable es el único valor de delivery_status observable por esta
// API: el Persistidor solo expone documentos que ya asentó.
const deliveryAvailable = "AVAILABLE"

// DocumentResponse es el DTO de salida: expone la identidad de negocio,
// nunca el `_id` interno de Mongo.
type DocumentResponse struct {
	DocumentID       string          `json:"document_id"`
	Filename         string          `json:"filename"`
	MimeType         string          `json:"mime_type"`
	Checksum         string          `json:"checksum"`
	DeliveryStatus   string          `json:"delivery_status"`
	SummaryStatus    string          `json:"summary_status"`
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
		Filename:         doc.Metadata.Filename,
		MimeType:         doc.Metadata.MimeType,
		Checksum:         doc.Checksum,
		DeliveryStatus:   deliveryAvailable,
		SummaryStatus:    string(doc.SummaryStatus),
		ExtractedText:    doc.ExtractedText,
		Summary:          doc.Summary,
		Metadata:         doc.Metadata,
		ProcessingTimeMS: doc.ExtractionTimeMS + doc.SummaryTimeMS,
		CreatedAt:        doc.CreatedAt,
		UpdatedAt:        doc.UpdatedAt,
	}
}
