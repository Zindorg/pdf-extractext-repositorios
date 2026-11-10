package redis

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
)

// MessageHandlers aplica los eventos del stream como casos de uso de dominio.
type MessageHandlers struct {
	service *application.DocumentService
}

func NewMessageHandlers(service *application.DocumentService) *MessageHandlers {
	return &MessageHandlers{service: service}
}

// OnOriginal crea el documento pendiente. En entrega at-least-once un evento
// repetido es legítimo: se descarta silenciosamente (nil ⇒ ACK), no se
// reintenta.
func (h *MessageHandlers) OnOriginal(ctx context.Context, ev *OriginalEvent) error {
	if err := verifyChecksum(ev.ExtractedText, ev.Checksum); err != nil {
		return err
	}
	extractedAt := ev.ExtractedAt
	if extractedAt == nil || extractedAt.IsZero() {
		now := domain.Now()
		extractedAt = &now
	}
	_, err := h.service.CreatePending(ctx, domain.Document{
		DocumentID:       ev.DocumentID,
		Checksum:         ev.Checksum,
		SummaryStatus:    summaryStatusFrom(ev),
		ExtractedText:    ev.ExtractedText,
		ExtractionTimeMS: ev.ExtractionTimeMS,
		ExtractedAt:      extractedAt,
		Metadata: domain.Metadata{
			Filename:  filenameOf(ev),
			SizeBytes: ev.Metadata.SizeBytes,
			PageCount: ev.Metadata.PageCount,
			MimeType:  ev.MimeType,
		},
	})
	return swallowDuplicate(err)
}

// verifyChecksum comprueba que el checksum proporcionado coincide con
// SHA256(extracted_text) en minúsculas (formato del orquestador).
func verifyChecksum(extractedText, checksum string) error {
	h := sha256.Sum256([]byte(extractedText))
	computed := fmt.Sprintf("%x", h)
	if computed != checksum {
		return fmt.Errorf("%w: expected %s, got %s", domain.ErrInvalidRequest, computed, checksum)
	}
	return nil
}

// summaryStatusFrom normaliza el eje de resumen al ingerir. El original puede
// declarar RESOLVED porque el resumen ya se publicó, pero hasta que llegue su
// texto el documento sigue pendiente; FAILED no tiene texto por venir y se
// conserva.
func summaryStatusFrom(ev *OriginalEvent) domain.SummaryStatus {
	declared := domain.SummaryStatus(ev.SummaryStatus)
	switch declared {
	case domain.SummaryNotRequested, domain.SummaryFailed:
		return declared
	case domain.SummaryPending, domain.SummaryResolved:
		return domain.SummaryPending
	}
	if ev.SummaryRequested {
		return domain.SummaryPending
	}
	return domain.SummaryNotRequested
}

// filenameOf prefiere el filename de primer nivel del evento y cae al que
// declare metadata, en ese orden.
func filenameOf(ev *OriginalEvent) string {
	if ev.Filename != "" {
		return ev.Filename
	}
	return ev.Metadata.Filename
}

// OnSummaryResolved completa el documento con el resumen. Un resumen de un
// documento inexistente (huérfano) es un error real que irá a la DLQ tras
// reintentos; una re-entrega sobre un documento ya completado es benigna.
func (h *MessageHandlers) OnSummaryResolved(ctx context.Context, ev *SummaryResolvedEvent) error {
	resolvedAt := ev.ResolvedAt
	if resolvedAt == nil || resolvedAt.IsZero() {
		now := domain.Now()
		resolvedAt = &now
	}
	_, err := h.service.CompleteWithSummary(ctx, ev.DocumentID, ev.Summary, ev.SummaryTimeMS, resolvedAt)
	return err
}

func swallowDuplicate(err error) error {
	if errors.Is(err, domain.ErrDuplicateDocumentID) || errors.Is(err, domain.ErrDuplicateChecksum) {
		slog.Debug("duplicate event ignored", "error", err)
		return nil
	}
	return err
}
