package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// handleMessage valida el envoltorio, rootea por el discriminador y aplica el
// handler correspondiente; devuelve el error de negocio (no ACK) del mensaje.
func (c *StreamConsumer) handleMessage(ctx context.Context, msg redis.XMessage) error {
	payload, ok := msg.Values[streamPayloadField].(string)
	if !ok {
		return fmt.Errorf("%s: missing field %q", c.streamName, streamPayloadField)
	}
	kind, err := discriminator(payload)
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	switch kind {
	case eventOriginal:
		return c.handleOriginal(ctx, payload)
	case eventSummaryResolved:
		return c.handleSummaryResolved(ctx, payload)
	default:
		return fmt.Errorf("stream: unsupported event kind %q", kind)
	}
}

func (c *StreamConsumer) handleOriginal(ctx context.Context, payload string) error {
	ev, err := parseOriginal(payload)
	if err != nil {
		return err
	}
	if err := ev.validateSize(c.maxTextBytes); err != nil {
		return err
	}
	return c.handlers.OnOriginal(ctx, ev)
}

func (c *StreamConsumer) handleSummaryResolved(ctx context.Context, payload string) error {
	ev, err := parseSummaryResolved(payload)
	if err != nil {
		return err
	}
	return c.handlers.OnSummaryResolved(ctx, ev)
}
