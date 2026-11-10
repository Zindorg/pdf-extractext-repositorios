package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/redis/go-redis/v9"
)

var (
	errNotConnected = errors.New("redis not connected")

	_ domain.StreamConsumer = (*StreamConsumer)(nil)
)

const (
	streamPayloadField = "payload"
	consumeBatch       = 10
	consumeBlock       = time.Second
)

// Config agrupa los ajustes del consumer del stream.
type Config struct {
	StreamName   string
	StreamGroup  string
	DLQName      string
	ConsumerName string
	MaxTextBytes int64
}

// StreamConsumer consume el stream document-events con un consumer group
// (entrega at-least-once): los mensajes nuevos se leen con XREADGROUP y los
// fallos quedan pending, se reclaman con XAUTOCLAIM tras el backoff y van a
// la DLQ cuando se agotan los reintentos.
type StreamConsumer struct {
	client       *redis.Client
	streamName   string
	streamGroup  string
	dlqName      string
	consumerName string
	maxTextBytes int64
	handlers     *MessageHandlers
	retry        *RetryDLQ
}

func NewStreamConsumer(
	client *redis.Client,
	cfg Config,
	service *application.DocumentService,
	retry *RetryDLQ,
) (*StreamConsumer, error) {
	if client == nil {
		return nil, errNotConnected
	}
	return &StreamConsumer{
		client:       client,
		streamName:   cfg.StreamName,
		streamGroup:  cfg.StreamGroup,
		dlqName:      cfg.DLQName,
		consumerName: cfg.ConsumerName,
		maxTextBytes: cfg.MaxTextBytes,
		handlers:     NewMessageHandlers(service),
		retry:        retry,
	}, nil
}

// Run ejecuta el loop de consumo hasta que ctx se cancela (retorna nil).
func (c *StreamConsumer) Run(ctx context.Context) error {
	if err := c.ensureGroup(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("stream: %w", err)
	}

	// Iniciar loop periódico de XTRIM MINID (cada minuto).
	go c.trimLoop(ctx)

	for {
		if err := c.consume(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

// consume procesa un lote: primero los pendientes que hayan superado el
// backoff (reintentos) y después los mensajes nuevos del grupo.
func (c *StreamConsumer) consume(ctx context.Context) error {
	if err := c.processClaimed(ctx); err != nil {
		return err
	}
	return c.processFresh(ctx)
}

// processClaimed recalca los mensajes fallidos (XAUTOCLAIM) que ya llevan
// más de un backoff pendientes.
func (c *StreamConsumer) processClaimed(ctx context.Context) error {
	messages, _, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   c.streamName,
		Group:    c.streamGroup,
		Consumer: c.consumerName,
		MinIdle:  c.retry.Backoff(),
		Start:    "-",
		Count:    consumeBatch,
	}).Result()
	if err != nil {
		return fmt.Errorf("xautoclaim: %w", err)
	}
	return c.processLote(ctx, messages)
}

// processFresh lee los mensajes no entregados del grupo (">") bloqueando
// hasta consumeBlock; no hay mensajes nuevos ⇒ redis.Nil, sin error.
func (c *StreamConsumer) processFresh(ctx context.Context) error {
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.streamGroup,
		Consumer: c.consumerName,
		Streams:  []string{c.streamName, ">"},
		Count:    consumeBatch,
		Block:    consumeBlock,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("xreadgroup: %w", err)
	}
	var messages []redis.XMessage
	for _, stream := range streams {
		messages = append(messages, stream.Messages...)
	}
	return c.processLote(ctx, messages)
}

// processLote aplica process a un lote; un fallo transitorio de Redis por
// mensaje se registra y se sigue (el mensaje queda pending y XAUTOCLAIM lo
// reintenta). Solo la cancelación de ctx propaga el error.
func (c *StreamConsumer) processLote(ctx context.Context, messages []redis.XMessage) error {
	for _, msg := range messages {
		if err := c.process(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return err // cancelación: Run la traduce en nil
			}
			slog.Error("stream message processing failed", "msg_id", msg.ID, "error", err)
		}
	}
	return nil
}

// process despacha un mensaje por su discriminador y decide ACK o fallo.
func (c *StreamConsumer) process(ctx context.Context, msg redis.XMessage) error {
	handleErr := c.handleMessage(ctx, msg)
	if handleErr != nil {
		return c.fail(ctx, msg, handleErr)
	}
	return c.ack(ctx, msg.ID)
}

// ensureGroup crea el consumer group (mkstream) si no existe; un grupo ya
// creado es un no-op (BUSYGROUP).
func (c *StreamConsumer) ensureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.streamName, c.streamGroup, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("xgroup create: %w", err)
	}
	return nil
}

// trimLoop ejecuta XTRIM MINID periódicamente para recortar el stream.
// Se ejecuta cada minuto. El umbral es el menor entre:
// - el ID pendiente más viejo del grupo (XPENDING)
// - now - 7 días (en ms)
// Si no hay pendientes, usa last-delivered-id del grupo (XINFO GROUPS).
// Nunca recorta mensajes sin entregar (pending) porque el umbral es <= al
// menor ID pendiente.
func (c *StreamConsumer) trimLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.trimStream(ctx); err != nil {
				slog.Debug("stream trim failed", "stream", c.streamName, "error", err)
			}
		}
	}
}

// trimStream calcula el umbral y ejecuta XTRIM MINID.
func (c *StreamConsumer) trimStream(ctx context.Context) error {
	// 1. Obtener el ID pendiente más viejo del grupo.
	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.streamName,
		Group:  c.streamGroup,
		Start:  "-",
		End:    "+",
		Count:  1,
	}).Result()
	if err != nil {
		return fmt.Errorf("xpending: %w", err)
	}

	var oldestPendingID string
	if len(pending) > 0 {
		oldestPendingID = pending[0].ID
	} else {
		// 2. Si no hay pendientes, usar last-delivered-id del grupo.
		groups, err := c.client.XInfoGroups(ctx, c.streamName).Result()
		if err != nil {
			return fmt.Errorf("xinfo groups: %w", err)
		}
		for _, g := range groups {
			if g.Name == c.streamGroup {
				oldestPendingID = g.LastDeliveredID
				break
			}
		}
	}

	// 3. Calcular umbral por tiempo (now - 7 días en ms).
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	timeThresholdID := fmt.Sprintf("%d-0", sevenDaysAgo)

	// 4. El umbral final es el menor entre oldestPendingID y timeThresholdID.
	// Los IDs de stream son <ms>-<seq>, comparar como string funciona porque ms es la parte principal.
	var trimID string
	if oldestPendingID != "" && oldestPendingID < timeThresholdID {
		trimID = oldestPendingID
	} else {
		trimID = timeThresholdID
	}

	// 5. Ejecutar XTRIM MINID ~ (aproximado para eficiencia).
	// El ~ evita recortar exactamente en el límite y es más eficiente.
	if err := c.client.XTrimMinID(ctx, c.streamName, trimID).Err(); err != nil {
		return fmt.Errorf("xtrim minid: %w", err)
	}
	slog.Debug("stream trimmed", "stream", c.streamName, "min_id", trimID)
	return nil
}
