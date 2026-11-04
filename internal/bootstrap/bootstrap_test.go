package bootstrap

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"serv_tpl/internal/global"
)

// async 让测试 goroutine 的意外 panic 可观察。
func async(t *testing.T, fn func()) {
	t.Helper()
	go func() {
		defer func() {
			if v := recover(); v != nil {
				t.Errorf("测试 goroutine panic: %v", v)
			}
		}()
		fn()
	}()
}
func TestLogInit(t *testing.T) {
	old := global.AppConf
	defer func() { global.AppConf = old }()
	for _, medium := range []string{"stdout", "file"} {
		global.AppConf = viper.New()
		global.AppConf.Set("log.type", medium)
		global.AppConf.Set("log.path", t.TempDir()+"/nested/app.log")
		stop := MustInitLog()
		require.NoError(t, stop(context.Background()))
		require.NoError(t, stop(context.Background()))
	}
	global.AppConf = viper.New()
	global.AppConf.Set("log.type", "file")
	require.Panics(t, func() { MustInitLog() })
	global.AppConf = viper.New()
	stop := MustInitLog()
	require.NoError(t, stop(context.Background()))
}
