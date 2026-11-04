package global

import (
	"fmt"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGRPCConfig(t *testing.T) {
	c := viper.New()
	cfg, err := LoadLifecycleConfig(c)
	require.NoError(t, err)
	require.False(t, cfg.GRPC.Enabled)
	c.Set("grpc.enable", true)
	cfg, err = LoadLifecycleConfig(c)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:9090", cfg.GRPC.Address)
	require.Equal(t, 5*time.Second, cfg.GRPC.Drain)
	require.Equal(t, 5*time.Second, cfg.GRPC.ConnectionTimeout)
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"grpc.enable", "invalid"}, {"grpc.address", "invalid"},
		{"grpc.address", "localhost:0"}, {"grpc.address", "localhost:65536"},
		{"grpc.drain_timeout", "invalid"}, {"grpc.drain_timeout", "0s"},
		{"grpc.drain_timeout", "10s"}, {"grpc.connection_timeout", "-1s"},
	} {
		t.Run(tc.key+"/"+fmt.Sprint(tc.value), func(t *testing.T) {
			v := viper.New()
			v.Set("grpc.enable", true)
			v.Set(tc.key, tc.value)
			_, err := LoadLifecycleConfig(v)
			require.Error(t, err)
		})
	}
}

func TestUnusedGRPCConfigIgnored(t *testing.T) {
	c := viper.New()
	for _, key := range []string{"grpc.address", "grpc.drain_timeout", "grpc.connection_timeout"} {
		c.Set(key, "invalid")
	}
	_, err := LoadLifecycleConfig(c)
	require.NoError(t, err)
	c.Set("grpc.enable", "invalid")
	cfg, err := LoadWorkerLifecycleConfig(c)
	require.NoError(t, err)
	require.False(t, cfg.GRPC.Enabled)
}
