package domain

import "time"

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusCompleted Status = "COMPLETED"
)

type Metadata struct {
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	PageCount int    `json:"page_count"`
}

type Document struct {
	DocumentID       string     `json:"document_id"` // UUID — clave de negocio
	Checksum         string     `json:"checksum"`    // sha256(extracted_text)
	Status           Status     `json:"status"`
	ExtractedText    string     `json:"extracted_text"`
	Summary          *string    `json:"summary"` // null mientras PENDING
	Metadata         Metadata   `json:"metadata"`
	ExtractionTimeMS int64      `json:"extraction_time_ms"`
	SummaryTimeMS    int64      `json:"summary_time_ms"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at"` // soft-delete
}

// Now devuelve la hora canónica del servicio (UTC). Todas las marcas de
// tiempo que muta el servicio (created_at, updated_at) salen de este reloj:
// service, adaptadores y fakes de test lo comparten.
func Now() time.Time { return time.Now().UTC() }

func (d Document) IsCompleted() bool {
	return d.Status == StatusCompleted
}

func (d Document) IsDeleted() bool {
	return d.DeletedAt != nil
}
