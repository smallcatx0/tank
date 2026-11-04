package global

import (
	"fmt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLifecycleDefaults(t *testing.T) {
	o, e := LoadLifecycleConfig(viper.New())
	require.NoError(t, e)
	require.Equal(t, 8080, o.Port)
	require.False(t, o.Debug)
}

func TestWorkerIgnoresHTTPConfig(t *testing.T) {
	for _, withInvalidHTTP := range []bool{false, true} {
		c := viper.New()
		// worker 的短关闭期限不受 HTTP 默认排空期限限制。
		c.Set("shutdown_timeout", 1)
		if withInvalidHTTP {
			for _, key := range []string{"http.port", "http.debug", "http.read_header_timeout", "http.read_timeout", "http.write_timeout", "http.idle_timeout", "http.drain_timeout"} {
				c.Set(key, "invalid")
			}
		}
		cfg, err := LoadWorkerLifecycleConfig(c)
		require.NoError(t, err)
		require.Zero(t, cfg.Port)
		require.Zero(t, cfg.Drain)
		if !withInvalidHTTP {
			require.False(t, c.IsSet("http.port"))
		}
		_, err = LoadLifecycleConfig(c)
		require.Error(t, err, "API 入口仍须校验 HTTP 参数")
	}
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"startup_timeout", "invalid"}, {"shutdown_timeout", 0}, {"metrics.enable", "invalid"}, {"log.type", "file"},
	} {
		c := viper.New()
		c.Set(tc.key, tc.value)
		_, err := LoadWorkerLifecycleConfig(c)
		require.Error(t, err)
	}
}
func TestLifecycleInvalid(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"http.port", 0}, {"http.port", -1}, {"http.port", 65536}, {"http.port", "bad"},
		{"shutdown_timeout", 0}, {"shutdown_timeout", "bad"}, {"http.read_timeout", "bad"},
		{"http.idle_timeout", "0s"}, {"startup_timeout", "-1s"}, {"http.drain_timeout", "30s"},
		{"http.debug", "bad"}, {"metrics.enable", "bad"}, {"shutdown_timeout", int64(9223372036854775807)},
	} {
		t.Run(tc.key+":"+fmt.Sprint(tc.value), func(t *testing.T) {
			c := viper.New()
			c.Set(tc.key, tc.value)
			_, e := LoadLifecycleConfig(c)
			require.Error(t, e)
		})
	}
	c := viper.New()
	c.Set("metrics.enable", true)
	c.Set("metrics.dt", -1)
	_, e := LoadLifecycleConfig(c)
	require.Error(t, e)
	c.Set("metrics.dt", 10)
	_, e = LoadLifecycleConfig(c)
	require.NoError(t, e)
	c.Set("metrics.dt", int64(9223372036854775807))
	_, e = LoadLifecycleConfig(c)
	require.Error(t, e)
	c = viper.New()
	c.Set("log.type", "file")
	_, e = LoadLifecycleConfig(c)
	require.Error(t, e)
	c.Set("log.path", "file.log")
	_, e = LoadLifecycleConfig(c)
	require.NoError(t, e)
}
