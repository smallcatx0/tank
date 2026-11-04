package bootstrap

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"os"
	"serv_tpl/internal/global"
	"testing"
	"time"
)

func TestMustInitDBsIsolated(t *testing.T) {
	dsn := os.Getenv("D02_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("需要隔离 MySQL 的 D02_TEST_MYSQL_DSN")
	}
	old, db, r := global.AppConf, global.MysqlCli, global.RedisCli
	defer func() { global.AppConf, global.MysqlCli, global.RedisCli = old, db, r }()
	redis := miniredis.RunT(t)
	global.AppConf = viper.New()
	global.AppConf.Set("mysql.dsn", dsn)
	global.AppConf.Set("redis.addr", redis.Addr())
	closeDB := MustInitDBs(3 * time.Second)
	defer closeDB(context.Background())
	require.NoError(t, closeDB(context.Background()))
	require.NoError(t, closeDB(context.Background()))
	pool, err := global.MysqlCli.DB()
	require.NoError(t, err)
	require.Error(t, pool.PingContext(context.Background()))
	require.Error(t, global.RedisCli.Ping(context.Background()).Err())
}
