package redis

import (
	"context"
	"errors"

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
	_, err := h.service.CreatePending(ctx, domain.Document{
		DocumentID:       ev.DocumentID,
		Checksum:         ev.Checksum,
		ExtractedText:    ev.ExtractedText,
		ExtractionTimeMS: ev.ExtractionTimeMS,
		Metadata: domain.Metadata{
			Filename:  ev.Metadata.Filename,
			SizeBytes: ev.Metadata.SizeBytes,
			PageCount: ev.Metadata.PageCount,
			MimeType:  ev.MimeType,
		},
	})
	return swallowDuplicate(err)
}

// OnSummaryResolved completa el documento con el resumen. Un resumen de un
// documento inexistente (huérfano) es un error real que irá a la DLQ tras
// reintentos; una re-entrega sobre un documento ya completado es benigna.
func (h *MessageHandlers) OnSummaryResolved(ctx context.Context, ev *SummaryResolvedEvent) error {
	_, err := h.service.CompleteWithSummary(ctx, ev.DocumentID, ev.Summary, ev.SummaryTimeMS)
	return err
}

func swallowDuplicate(err error) error {
	if errors.Is(err, domain.ErrDuplicateDocumentID) || errors.Is(err, domain.ErrDuplicateChecksum) {
		return nil
	}
	return err
}
