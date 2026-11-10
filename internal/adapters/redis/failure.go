package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"github.com/redis/go-redis/v9"
)

// fail cuenta el intento y decide: si quedan reintentos deja el mensaje
// pending (XAUTOCLAIM lo retomará tras el backoff); si se agotan, lo envía a
// la DLQ y lo ACK.
func (c *StreamConsumer) fail(ctx context.Context, msg redis.XMessage, cause error) error {
	attempts, err := c.client.HIncrBy(ctx, c.retryCountKey(), msg.ID, 1).Result()
	if err != nil {
		return fmt.Errorf("hincrex retries: %w", err)
	}
	if !c.retry.NeedsDLQ(int(attempts)) {
		return nil
	}
	if err := c.pushDLQ(ctx, msg, cause.Error(), attempts); err != nil {
		return err
	}
	if err := c.client.HDel(ctx, c.retryCountKey(), msg.ID).Err(); err != nil {
		return fmt.Errorf("hdel retries: %w", err)
	}
	return c.ack(ctx, msg.ID)
}

func (c *StreamConsumer) pushDLQ(ctx context.Context, msg redis.XMessage, cause string, attempts int64) error {
	original, err := json.Marshal(msg.Values)
	if err != nil {
		return fmt.Errorf("marshal dlq message: %w", err)
	}
	_, err = c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: c.dlqName,
		Values: map[string]any{
			"message":   string(original),
			"error":     cause,
			"attempts":  attempts,
			"failed_at": domain.Now().Format(time.RFC3339),
		},
	}).Result()
	if err != nil {
		return fmt.Errorf("xadd dlq: %w", err)
	}
	return nil
}

func (c *StreamConsumer) ack(ctx context.Context, messageID string) error {
	if err := c.client.XAck(ctx, c.streamName, c.streamGroup, messageID).Err(); err != nil {
		return fmt.Errorf("xack: %w", err)
	}
	return nil
}

// retryCountKey es la clave del hash que lleva los reintentos por mensaje.
func (c *StreamConsumer) retryCountKey() string {
	return c.streamName + ":retries"
}
