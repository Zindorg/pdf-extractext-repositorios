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
	consumerName       = "persister"
)

// StreamConsumer consume el stream document-events con un consumer group
// (entrega at-least-once): los mensajes nuevos se leen con XREADGROUP y los
// fallos quedan pending, se reclaman con XAUTOCLAIM tras el backoff y van a
// la DLQ cuando se agotan los reintentos.
type StreamConsumer struct {
	client       *redis.Client
	streamName   string
	streamGroup  string
	dlqName      string
	maxTextBytes int64
	handlers     *MessageHandlers
	retry        *RetryDLQ
}

func NewStreamConsumer(
	client *redis.Client,
	streamName, streamGroup, dlqName string,
	service *application.DocumentService,
	retry *RetryDLQ,
	maxTextBytes int64,
) (*StreamConsumer, error) {
	if client == nil {
		return nil, errNotConnected
	}
	return &StreamConsumer{
		client:       client,
		streamName:   streamName,
		streamGroup:  streamGroup,
		dlqName:      dlqName,
		maxTextBytes: maxTextBytes,
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
		Consumer: consumerName,
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
		Consumer: consumerName,
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
