package redis

import (
	"context"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
)

// MessageHandlers desacopla el consumer de los casos de uso:

type MessageHandlers struct {
	service *application.DocumentService
}

func NewMessageHandlers(service *application.DocumentService) *MessageHandlers {
	return &MessageHandlers{service: service}
}

// OnOriginal procesa el evento "original". La lógica (insert PENDING +
// dedup) se completa en la fase de lógica.
func (h *MessageHandlers) OnOriginal(_ context.Context, _ *OriginalEvent) error {
	return errNotImplemented
}

// OnSummary procesa el evento "summary". La lógica (multiplicado a COMPLETED)
// se completa en la fase de lógica.
func (h *MessageHandlers) OnSummary(_ context.Context, _ *SummaryEvent) error {
	return errNotImplemented
}
