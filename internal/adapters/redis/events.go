package redis

import (
	"encoding/json"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// OriginalEvent transporta el texto extraído (evento "original").
type OriginalEvent struct {
	Event            string          `json:"event"`
	DocumentID       string          `json:"document_id"`
	Checksum         string          `json:"checksum"`
	ExtractedText    string          `json:"extracted_text"`
	Metadata         domain.Metadata `json:"metadata"`
	ProcessingTimeMS int64           `json:"processing_time_ms"`
}

// SummaryEvent transporta el resumen (evento "summary").
type SummaryEvent struct {
	Event      string `json:"event"`
	DocumentID string `json:"document_id"`
	Summary    string `json:"summary"`
}

func parseOriginal(data string) (*OriginalEvent, error) {
	var ev OriginalEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func parseSummary(data string) (*SummaryEvent, error) {
	var ev SummaryEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// MessageMetadata agrupa los datos fijos de contacto con Redis/stream.
type MessageMetadata struct {
	StreamName string
	Group      string
	Now        func() time.Time
}

func defaultMessageMetadata(stream, group string) MessageMetadata {
	return MessageMetadata{StreamName: stream, Group: group, Now: time.Now}
}
