package redis

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

const (
	schemaVersion = 1

	eventOriginal        = "original"
	eventSummaryResolved = "summary_resolved"
)

// OriginalEvent transporta el extracto del documento (evento "original").
// SummaryRequested y SummaryStatus son informativos del orquestador: se
// parsean pero no se validan ni se persisten.
type OriginalEvent struct {
	EventType        string          `json:"event_type"`
	Event            string          `json:"event"`
	SchemaVersion    int             `json:"schema_version"`
	DocumentID       string          `json:"document_id"`
	Filename         string          `json:"filename"`
	Checksum         string          `json:"checksum"`
	ExtractedText    string          `json:"extracted_text"`
	ExtractionTimeMS int64           `json:"extraction_time_ms"`
	MimeType         string          `json:"mime_type"`
	SummaryRequested bool            `json:"summary_requested"`
	SummaryStatus    string          `json:"summary_status"`
	ExtractedAt      *time.Time      `json:"extracted_at,omitempty"` // RFC3339 UTC — momento real del evento
	Metadata         domain.Metadata `json:"metadata"`
}

// SummaryResolvedEvent transporta el resumen (evento "summary_resolved").
type SummaryResolvedEvent struct {
	EventType     string     `json:"event_type"`
	Event         string     `json:"event"`
	SchemaVersion int        `json:"schema_version"`
	DocumentID    string     `json:"document_id"`
	Summary       string     `json:"summary"`
	SummaryTimeMS int64      `json:"summary_time_ms"`
	SummaryStatus string     `json:"summary_status"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"` // RFC3339 UTC — momento real del evento
}

// discriminator resuelve el tipo de evento. El orquestador envía event_type
// (nuevo) con event (legado) indistintamente: si solo viene uno, se usa; si
// vienen ambos deben coincidir. No valida el valor: el routing decide.
func discriminator(data string) (string, error) {
	var p struct {
		EventType string `json:"event_type"`
		Event     string `json:"event"`
	}
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return "", err
	}
	return resolveDiscriminator(p.EventType, p.Event)
}

func resolveDiscriminator(eventType, event string) (string, error) {
	switch {
	case eventType != "" && event != "":
		if eventType != event {
			return "", fmt.Errorf("discriminator mismatch: event_type=%q event=%q", eventType, event)
		}
		return eventType, nil
	case eventType != "":
		return eventType, nil
	case event != "":
		return event, nil
	default:
		return "", errors.New("missing event discriminator")
	}
}

// parseEvent decodifica el payload en el tipo de evento esperado y lo valida.
// kind es el discriminante que el payload debe resolver.
func parseEvent[T any](data, kind string, validate func(*T) error) (*T, error) {
	resolved, err := discriminator(data)
	if err != nil {
		return nil, err
	}
	if resolved != kind {
		return nil, fmt.Errorf("expected %q event, got %q", kind, resolved)
	}
	var ev T
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil, err
	}
	if err := validate(&ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func parseOriginal(data string) (*OriginalEvent, error) {
	return parseEvent(data, eventOriginal, (*OriginalEvent).validate)
}

func parseSummaryResolved(data string) (*SummaryResolvedEvent, error) {
	return parseEvent(data, eventSummaryResolved, (*SummaryResolvedEvent).validate)
}

func (e *OriginalEvent) validate() error {
	switch {
	case e.SchemaVersion != schemaVersion:
		return fmt.Errorf("%s: unsupported schema_version %d", eventOriginal, e.SchemaVersion)
	case e.DocumentID == "":
		return errors.New(eventOriginal + ": document_id is required")
	case e.Checksum == "":
		return errors.New(eventOriginal + ": checksum is required")
	case e.ExtractedText == "":
		return errors.New(eventOriginal + ": extracted_text is required")
	case e.MimeType == "":
		return errors.New(eventOriginal + ": mime_type is required")
	default:
		return nil
	}
}

// validateSize aplica el límite de bytes al extracto extraído; el tope es
// inclusivo (un extracto de exactamente max bytes se acepta).
func (e *OriginalEvent) validateSize(max int64) error {
	if n := int64(len(e.ExtractedText)); n > max {
		return fmt.Errorf("%w: %d > %d bytes", domain.ErrTextTooLarge, n, max)
	}
	return nil
}

func (e *SummaryResolvedEvent) validate() error {
	switch {
	case e.SchemaVersion != schemaVersion:
		return fmt.Errorf("%s: unsupported schema_version %d", eventSummaryResolved, e.SchemaVersion)
	case e.DocumentID == "":
		return errors.New(eventSummaryResolved + ": document_id is required")
	case e.Summary == "":
		return errors.New(eventSummaryResolved + ": summary is required")
	default:
		return nil
	}
}

// parseTimeTolerant intenta parsear una fecha RFC3339. Si falla, loguea
// un warning y devuelve nil (no falla el mensaje).
func parseTimeTolerant(field, value string) *time.Time {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		slog.Warn("failed to parse event timestamp", "field", field, "value", value, "error", err)
		return nil
	}
	return &t
}

// UnmarshalJSON implementa parsing tolerante de extracted_at.
func (e *OriginalEvent) UnmarshalJSON(data []byte) error {
	type Alias OriginalEvent
	aux := &struct {
		ExtractedAt string `json:"extracted_at"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	e.ExtractedAt = parseTimeTolerant("extracted_at", aux.ExtractedAt)
	return nil
}

// UnmarshalJSON implementa parsing tolerante de resolved_at.
func (e *SummaryResolvedEvent) UnmarshalJSON(data []byte) error {
	type Alias SummaryResolvedEvent
	aux := &struct {
		ResolvedAt string `json:"resolved_at"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	e.ResolvedAt = parseTimeTolerant("resolved_at", aux.ResolvedAt)
	return nil
}
