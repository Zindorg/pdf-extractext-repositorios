package redis

import (
	"context"
	"errors"

	"github.com/marco/pdf-extractext-repositorios/internal/application"
	"github.com/redis/go-redis/v9"
)

var (
	errNotConnected   = errors.New("redis not connected")
	errNotImplemented = errors.New("not implemented yet")
)

// StreamConsumer consume el stream document-events con un consumer group.
// En esta fase es un esqueleto con wiring real (cliente Redis); el loop de
// XREADGROUP/XACK/XAUTOCLAIM se completa en la fase de lógica.
type StreamConsumer struct {
	client      *redis.Client
	streamName  string
	streamGroup string
	service     *application.DocumentService
	ack         func(messageIDs ...string) error
}

func NewStreamConsumer(
	ctx context.Context,
	client *redis.Client,
	streamName, streamGroup string,
	service *application.DocumentService,
) (*StreamConsumer, error) {
	return &StreamConsumer{
		client:      client,
		streamName:  streamName,
		streamGroup: streamGroup,
		service:     service,
		ack:         nil, // se provee en la fase de lógica (dependency-injection point)
	}, nil
}

// Run ejecuta el loop de consumo hasta que ctx se cancela.
func (c *StreamConsumer) Run(ctx context.Context) error {
	if c.client == nil {
		return errNotConnected
	}
	return nil
}
