package dao

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRedisLifecycle(t *testing.T) {
	srv := miniredis.RunT(t)
	cli, e := OpenRedis(&redis.Options{Addr: srv.Addr()})
	require.NoError(t, e)
	require.NoError(t, CloseRedis(cli))
	require.Error(t, cli.Ping(context.Background()).Err())
	_, e = OpenRedisContext(context.Background(), nil)
	require.Error(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = OpenRedisContext(ctx, &redis.Options{Addr: srv.Addr()})
	require.ErrorIs(t, e, context.Canceled)
	srv.RequireAuth("secret")
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, e = OpenRedisContext(ctx, &redis.Options{Addr: srv.Addr(), Password: "wrong-secret", MaxRetries: -1})
	require.Error(t, e)
	require.NotContains(t, e.Error(), "secret")
	require.NoError(t, CloseRedis(nil))
}
