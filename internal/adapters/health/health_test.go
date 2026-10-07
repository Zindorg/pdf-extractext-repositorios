package health

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

type fakeMongo struct{ err error }

func (f fakeMongo) Ping(context.Context, *readpref.ReadPref) error { return f.err }

type fakeRedis struct{ err error }

func (f fakeRedis) Ping(context.Context) *redis.StatusCmd {
	return redis.NewStatusResult("PONG", f.err)
}

func TestPing_OK(t *testing.T) {
	p := NewPinger(fakeMongo{}, fakeRedis{})

	require.NoError(t, p.Ping())
}

func TestPing_MongoDown(t *testing.T) {
	p := NewPinger(fakeMongo{err: errors.New("mongo down")}, fakeRedis{})

	require.Error(t, p.Ping())
}

func TestPing_RedisDown(t *testing.T) {
	p := NewPinger(fakeMongo{}, fakeRedis{err: errors.New("redis down")})

	require.Error(t, p.Ping())
}
