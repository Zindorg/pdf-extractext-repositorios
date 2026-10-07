package redis

import (
	"encoding/json"
	"errors"
	"fmt"

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
	Checksum         string          `json:"checksum"`
	ExtractedText    string          `json:"extracted_text"`
	ExtractionTimeMS int64           `json:"extraction_time_ms"`
	MimeType         string          `json:"mime_type"`
	SummaryRequested bool            `json:"summary_requested"`
	SummaryStatus    string          `json:"summary_status"`
	Metadata         domain.Metadata `json:"metadata"`
}

// SummaryResolvedEvent transporta el resumen (evento "summary_resolved").
type SummaryResolvedEvent struct {
	EventType     string `json:"event_type"`
	Event         string `json:"event"`
	SchemaVersion int    `json:"schema_version"`
	DocumentID    string `json:"document_id"`
	Summary       string `json:"summary"`
	SummaryTimeMS int64  `json:"summary_time_ms"`
	SummaryStatus string `json:"summary_status"`
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

func parseOriginal(data string) (*OriginalEvent, error) {
	kind, err := discriminator(data)
	if err != nil {
		return nil, err
	}
	if kind != eventOriginal {
		return nil, fmt.Errorf("expected %q event, got %q", eventOriginal, kind)
	}
	var ev OriginalEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil, err
	}
	if err := ev.validate(); err != nil {
		return nil, err
	}
	return &ev, nil
}

func parseSummaryResolved(data string) (*SummaryResolvedEvent, error) {
	kind, err := discriminator(data)
	if err != nil {
		return nil, err
	}
	if kind != eventSummaryResolved {
		return nil, fmt.Errorf("expected %q event, got %q", eventSummaryResolved, kind)
	}
	var ev SummaryResolvedEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil, err
	}
	if err := ev.validate(); err != nil {
		return nil, err
	}
	return &ev, nil
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
