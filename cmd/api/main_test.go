package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/pkg/logger"
)

// 配置和全局实例只在当前测试进程使用；这些测试不能并行运行。
func entryConfig(t *testing.T, config string) string {
	t.Helper()
	old, param := global.AppConf, global.Param
	db, redis := global.MysqlCli, global.RedisCli
	t.Cleanup(func() {
		global.AppConf, global.Param = old, param
		global.MysqlCli, global.RedisCli = db, redis
		h, err := logger.NewStdLogger("info")
		require.NoError(t, err)
		logger.SetGlobal(h)
	})
	dir := t.TempDir()
	logPath := filepath.ToSlash(filepath.Join(dir, "app.log"))
	file := filepath.Join(dir, "app.yaml")
	require.NoError(t, os.WriteFile(file, []byte("log:\n  type: file\n  path: '"+logPath+"'\n"+config), 0600))
	global.AppConf = viper.New()
	global.Param.C = file
	return logPath
}

func TestStartFailures(t *testing.T) {
	for _, tc := range []struct {
		name, config, want string
		cancel, logCreated bool
	}{
		{"invalid-http", "http:\n  port: 0\n", "http.port", false, false},
		{"invalid-common", "startup_timeout: invalid\n", "startup_timeout", false, false},
		{"canceled", "", "context canceled", true, true},
		{"invalid-dsn", "mysql:\n  dsn: invalid\n", "mysql DSN 无效", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logPath := entryConfig(t, tc.config)
			var startErr error
			code := bootstrap.Run(func(ctx context.Context, r *bootstrap.Runtime) error {
				if tc.cancel {
					canceled, cancel := context.WithCancel(ctx)
					cancel()
					ctx = canceled
				}
				startErr = start(ctx, r)
				return startErr
			})
			require.Equal(t, 1, code)
			require.ErrorContains(t, startErr, tc.want)
			if tc.logCreated {
				// 切割 writer 没有 Sync 方法；直接写入已关闭的 writer 应失败。
				require.Error(t, logger.L().Core().Write(zapcore.Entry{Level: zapcore.InfoLevel, Message: "after-close-probe"}, nil), "退出后日志 writer 必须关闭")
			} else {
				_, err := os.Stat(logPath)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestStartConfigPanic(t *testing.T) {
	entryConfig(t, "")
	global.Param.C = filepath.Join(t.TempDir(), "missing.yaml")
	require.Equal(t, 1, bootstrap.Run(start))
}

// TestStartIsolated 只连接显式指定的专用 MySQL；Redis 使用进程内隔离实例。
func TestStartIsolated(t *testing.T) {
	dsn := os.Getenv("D02_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("需要专用隔离 MySQL 的 D02_TEST_MYSQL_DSN")
	}
	for _, occupied := range []bool{false, true} {
		t.Run(fmt.Sprintf("port-occupied=%t", occupied), func(t *testing.T) {
			redis := miniredis.RunT(t)
			listener, err := net.Listen("tcp", ":0")
			require.NoError(t, err)
			defer listener.Close()
			port := listener.Addr().(*net.TCPAddr).Port
			if !occupied {
				require.NoError(t, listener.Close())
			}
			entryConfig(t, fmt.Sprintf("mysql:\n  dsn: '%s'\nredis:\n  addr: '%s'\nhttp:\n  port: %d\nmetrics:\n  enable: true\n  dt: 1\n", dsn, redis.Addr(), port))
			var startErr, requestErr error
			var status int
			code := bootstrap.Run(func(ctx context.Context, r *bootstrap.Runtime) error {
				startErr = start(ctx, r)
				if startErr != nil {
					return startErr
				}
				logger.L().Info("entry-started")
				client := &http.Client{Timeout: time.Second}
				resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
				requestErr = err
				if err == nil {
					status = resp.StatusCode
					_ = resp.Body.Close()
				}
				client.CloseIdleConnections()
				// 主动故障退出验证真实装配资源的回收；正常信号退出由 lifecycle 测试覆盖。
				r.Fail()
				return nil
			})
			require.Equal(t, 1, code)
			if occupied {
				require.Error(t, startErr)
			} else {
				require.NoError(t, startErr)
				require.NoError(t, requestErr)
				require.Equal(t, http.StatusOK, status)
				probe, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
				require.NoError(t, err, "退出后 HTTP 监听必须释放")
				_ = probe.Close()
			}
			require.NotNil(t, global.MysqlCli)
			require.NotNil(t, global.RedisCli)
			pool, err := global.MysqlCli.DB()
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			require.Error(t, pool.PingContext(ctx), "退出后 MySQL 必须关闭")
			require.Error(t, global.RedisCli.Ping(ctx).Err(), "退出后 Redis 必须关闭")
			require.Error(t, logger.L().Core().Write(zapcore.Entry{Level: zapcore.InfoLevel, Message: "after-close-probe"}, nil), "退出后日志 writer 必须关闭")
		})
	}
}
