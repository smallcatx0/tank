package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/pkg/logger"
)

func TestStartWorkerConfigAndRollback(t *testing.T) {
	for _, tc := range []struct {
		name, config, want string
		cancel, logCreated bool
	}{
		{"missing-http", "shutdown_timeout: 1\n", "context canceled", true, true},
		{"invalid-http", "shutdown_timeout: 1\nhttp:\n  port: invalid\n  drain_timeout: invalid\n", "context canceled", true, true},
		{"invalid-common", "shutdown_timeout: 0\n", "shutdown_timeout", true, false},
		{"invalid-dsn", "http:\n  port: invalid\nmysql:\n  dsn: invalid\n", "mysql DSN 无效", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, param := global.AppConf, global.Param
			t.Cleanup(func() {
				global.AppConf, global.Param = old, param
				h, err := logger.NewStdLogger("info")
				require.NoError(t, err)
				logger.SetGlobal(h)
			})
			dir := t.TempDir()
			logPath := filepath.ToSlash(filepath.Join(dir, "worker.log"))
			file := filepath.Join(dir, "worker.yaml")
			require.NoError(t, os.WriteFile(file, []byte("log:\n  type: file\n  path: '"+logPath+"'\n"+tc.config), 0600))
			global.AppConf = viper.New()
			global.Param.C = file
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
				require.Error(t, logger.L().Core().Write(zapcore.Entry{Level: zapcore.InfoLevel, Message: "after-close-probe"}, nil), "退出后日志 writer 必须关闭")
			} else {
				_, err := os.Stat(logPath)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}
