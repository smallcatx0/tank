package global

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestObservabilityConfiguration(t *testing.T) {
	for _, metrics := range []bool{false, true} {
		for _, tracing := range []bool{false, true} {
			c := viper.New()
			c.Set("observability.metrics.enable", metrics)
			c.Set("observability.tracing.enable", tracing)
			c.Set("observability.service_name", "test")
			o, err := LoadObservabilityConfig(c)
			require.NoError(t, err)
			require.Equal(t, metrics, o.Metrics.Enable)
			require.Equal(t, tracing, o.Tracing.Enable)
			require.NoError(t, ValidateObservabilityBudget(o, 40*time.Second, 20*time.Second, 5*time.Second))
			if metrics || tracing {
				require.Error(t, ValidateObservabilityBudget(o, 25*time.Second, 20*time.Second, 5*time.Second))
			}
		}
	}
	cases := map[string]any{"metrics.enable": "bad", "tracing.enable": "bad", "metrics.address": "localhost:0", "metrics.shutdown_timeout": "-1s", "service_name": " ", "tracing.endpoint": "http://user:password@example.com/v1/traces", "tracing.sample_ratio": "NaN", "tracing.export_timeout": "999999999999999999999s", "tracing.shutdown_timeout": "0s"}
	for key, value := range cases {
		t.Run(key, func(t *testing.T) {
			c := viper.New()
			c.Set("observability.metrics.enable", true)
			c.Set("observability.tracing.enable", true)
			c.Set("observability.service_name", "test")
			c.Set("observability."+key, value)
			_, err := LoadObservabilityConfig(c)
			require.Error(t, err)
		})
	}
	for _, url := range []string{"ftp://example.com", "http://example.com?q=x", "http://example.com#x", "http://example.com:70000", "http://example.com:bad", "http://"} {
		c := viper.New()
		c.Set("observability.tracing.enable", true)
		c.Set("observability.service_name", "test")
		c.Set("observability.tracing.endpoint", url)
		_, err := LoadObservabilityConfig(c)
		require.Error(t, err)
	}
	c := viper.New()
	c.Set("observability.tracing.endpoint", "invalid")
	c.Set("observability.metrics.shutdown_timeout", "invalid")
	_, err := LoadObservabilityConfig(c)
	require.NoError(t, err)
}
