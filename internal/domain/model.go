package domain

import "time"

// SummaryStatus es el eje público del estado de resumen (openapi SummaryStatus):
// lo decide el Orquestador y el Persistidor lo conserva tal como el contrato
// lo define. delivery_status no se persiste: un documento existente ya es
// AVAILABLE.
type SummaryStatus string

const (
	SummaryNotRequested SummaryStatus = "NOT_REQUESTED"
	SummaryPending      SummaryStatus = "PENDING"
	SummaryResolved     SummaryStatus = "RESOLVED"
	SummaryFailed       SummaryStatus = "FAILED"
)

type Metadata struct {
	Filename  string `json:"filename"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	PageCount int    `json:"page_count"`
}

type Document struct {
	DocumentID       string        `json:"document_id"` // UUID — clave de negocio
	Checksum         string        `json:"checksum"`    // sha256(extracted_text)
	SummaryStatus    SummaryStatus `json:"summary_status"`
	ExtractedText    string        `json:"extracted_text"`
	Summary          *string       `json:"summary"` // null mientras PENDING
	Metadata         Metadata      `json:"metadata"`
	ExtractionTimeMS int64         `json:"extraction_time_ms"`
	SummaryTimeMS    int64         `json:"summary_time_ms"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
	DeletedAt        *time.Time    `json:"deleted_at"` // soft-delete
}

// Now devuelve la hora canónica del servicio (UTC). Todas las marcas de
// tiempo que muta el servicio (created_at, updated_at) salen de este reloj:
// service, adaptadores y fakes de test lo comparten.
func Now() time.Time { return time.Now().UTC() }

// HasSummary es la guarda de idempotencia del resumen: la presencia del texto
// manda, no el estado declarado por el orquestador.
func (d Document) HasSummary() bool {
	return d.Summary != nil
}

func (d Document) IsDeleted() bool {
	return d.DeletedAt != nil
}
