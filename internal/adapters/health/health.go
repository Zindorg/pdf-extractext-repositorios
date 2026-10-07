// Package health agrupa el chequeo de dependencias reales (Mongo y Redis)
// para el endpoint /health, fuera del composition root.
package health

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// pingTimeout es el budget de cada chequeo de /health.
const pingTimeout = 2 * time.Second

// mongoPinger y redisPinger son las dependencias mínimas de Pinger (DIP):
// los clientes concretos de Mongo y Redis las satisfacen.
type mongoPinger interface {
	Ping(ctx context.Context, rp *readpref.ReadPref) error
}

type redisPinger interface {
	Ping(ctx context.Context) *redis.StatusCmd
}

// Pinger verifica que Mongo y Redis respondan. Implementa api.HealthChecker.
type Pinger struct {
	mongo mongoPinger
	redis redisPinger
}

func NewPinger(m mongoPinger, r redisPinger) *Pinger {
	return &Pinger{mongo: m, redis: r}
}

func (p *Pinger) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := p.mongo.Ping(ctx, readpref.Primary()); err != nil {
		return err
	}
	return p.redis.Ping(ctx).Err()
}
