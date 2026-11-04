package dao

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

// TestIsolatedStorage 只接受显式提供的专用测试实例，不回退本机默认库。
func TestIsolatedStorage(t *testing.T) {
	dsn, addr := os.Getenv("D02_TEST_MYSQL_DSN"), os.Getenv("D02_TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("仅在显式 D02 隔离实例运行")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, e := OpenMysqlContext(ctx, MysqlOptions{DSN: dsn, Debug: true, MaxIdleConns: 1, MaxOpenConns: 2})
	require.NoError(t, e)
	require.NoError(t, db.WithContext(ctx).Exec("SELECT 1").Error)
	require.NoError(t, CloseMysql(db))
	pool, e := db.DB()
	require.NoError(t, e)
	require.Error(t, pool.PingContext(ctx))
	cli, e := OpenRedisContext(ctx, &redis.Options{Addr: addr})
	require.NoError(t, e)
	require.NoError(t, CloseRedis(cli))
	_, e = OpenMysqlContext(ctx, MysqlOptions{DSN: "d02_nonexistent:wrong@tcp(" + os.Getenv("D02_TEST_MYSQL_ADDR") + ")/gtank_test"})
	require.Error(t, e)
	require.NotContains(t, e.Error(), "wrong")
}
func TestMysqlInvalidAndCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := OpenMysqlContext(ctx, MysqlOptions{})
	require.ErrorIs(t, e, context.Canceled)
	_, e = OpenMysql(MysqlOptions{DSN: "invalid-dsn"})
	require.Error(t, e)
	require.NoError(t, CloseMysql(nil))
}
