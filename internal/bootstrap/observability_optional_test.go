package bootstrap

import (
	"context"
	"errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"net/http/httptest"
	"serv_tpl/internal/global"
	"serv_tpl/internal/telemetry"
	"testing"
	"time"
)

func TestOptionalObservationInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]any
		total  time.Duration
	}{
		{"disabled", nil, time.Second},
		{"invalid_heartbeat", map[string]any{"metrics.enable": "invalid"}, time.Second},
		{"invalid_interval", map[string]any{"metrics.enable": true, "metrics.dt": -1}, time.Second},
		{"invalid_metrics", map[string]any{"observability.metrics.enable": true, "observability.metrics.address": "invalid"}, time.Minute},
		{"invalid_tracing", map[string]any{"observability.tracing.enable": true}, time.Minute},
		{"insufficient_budget", map[string]any{"observability.metrics.enable": true}, time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := viper.New()
			for k, v := range tc.values {
				c.Set(k, v)
			}
			o := StartObservability(c, global.LifecycleConfig{Shutdown: tc.total})
			require.NotNil(t, o.Runtime)
			require.Nil(t, o.monitor)
			require.Nil(t, o.Runtime.Registry)
			require.False(t, o.Runtime.Options.Tracing.Enable)
			// 普通业务路由必须能使用降级后的 Runtime，保留 W3C 透传。
			server := MustStartHTTP(global.LifecycleConfig{Drain: time.Second}, o.Runtime)
			t.Cleanup(func() { require.NoError(t, server.Shutdown(context.Background())) })
			_, port, err := net.SplitHostPort(server.listener.Addr().String())
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+"/", nil)
			require.NoError(t, err)
			req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
			client := http.Client{Timeout: time.Second}
			resp, err := client.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", resp.Header.Get("x-b3-traceid"))
			require.NoError(t, resp.Body.Close())
			require.NoError(t, server.Shutdown(context.Background()))
			require.NoError(t, o.Shutdown(context.Background()))
			require.NoError(t, o.Shutdown(context.Background()))
		})
	}
}

func TestOptionalObservationInitializationFailure(t *testing.T) {
	o := startObservation(telemetry.Options{}, func(context.Context, telemetry.Options) (*telemetry.Runtime, error) {
		return nil, errors.New("private exporter detail")
	})
	require.NotNil(t, o.Runtime)
	require.Nil(t, o.Runtime.Registry)
	require.NoError(t, o.Shutdown(context.Background()))
}

func TestOptionalObservationPortConflictRetainsTracing(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	c := viper.New()
	c.Set("observability.service_name", "test")
	c.Set("observability.metrics.enable", true)
	c.Set("observability.metrics.address", listener.Addr().String())
	c.Set("observability.tracing.enable", true)
	// Collector 不可达不会阻止创建；无 span 时不产生导出请求。
	c.Set("observability.tracing.endpoint", "http://127.0.0.1:1/v1/traces")
	o := StartObservability(c, global.LifecycleConfig{Shutdown: time.Minute})
	require.Nil(t, o.monitor)
	require.Nil(t, o.Runtime.Registry)
	require.Nil(t, o.Runtime.Metrics)
	require.False(t, o.Runtime.Options.Metrics.Enable)
	require.True(t, o.Runtime.Options.Tracing.Enable)
	require.NoError(t, o.Shutdown(context.Background()))
}

func TestOptionalMonitorFailureAndShutdown(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "serve_failure"}[failure], func(t *testing.T) {
			o := startObservation(telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: true, Address: "127.0.0.1:0", ShutdownTimeout: time.Second}}, telemetry.New)
			client := http.Client{Timeout: time.Second}
			resp, err := client.Get("http://" + o.monitor.listener.Addr().String() + "/metrics")
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)
			require.NoError(t, resp.Body.Close())
			if failure {
				require.NoError(t, o.monitor.listener.Close())
				select {
				case <-o.watched:
				case <-time.After(time.Second):
					t.Fatal("监控故障观察协程未退出")
				}
			}
			require.NoError(t, o.Shutdown(context.Background()))
			require.NoError(t, o.Shutdown(context.Background()))
			select {
			case <-o.watched:
			default:
				t.Fatal("关闭后仍有监控观察协程")
			}
		})
	}
}

func TestOptionalTraceShutdownFailure(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	o := startObservation(telemetry.Options{ServiceName: "test", Tracing: telemetry.TracingOptions{Enable: true, Endpoint: server.URL + "/v1/traces", SampleRatio: 1, ExportTimeout: 100 * time.Millisecond, ShutdownTimeout: 10 * time.Millisecond}}, telemetry.New)
	_, span := o.Runtime.Tracer().Start(context.Background(), "shutdown")
	span.End()
	require.NoError(t, o.Shutdown(context.Background()), "观测关闭失败不得改变业务退出码")
}
