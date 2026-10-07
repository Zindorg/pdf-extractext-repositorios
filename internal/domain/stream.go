package domain

import "context"

// StreamConsumer consume los eventos del orquestador y aplica los
// consecuentes de dominio (crear pendiente, completar con resumen). Una
// única implementación (Redis streams) la satisface desde la app.
type StreamConsumer interface {
	Run(ctx context.Context) error
}
