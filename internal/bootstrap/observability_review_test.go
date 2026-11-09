package bootstrap

import (
	"context"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"net/http/httptest"
	"serv_tpl/internal/global"
	"serv_tpl/internal/telemetry"
	"strconv"
	"testing"
	"time"
)

func TestObservationReservesBusinessPorts(t *testing.T) {
	for _, business := range []string{"http", "grpc"} {
		t.Run(business, func(t *testing.T) {
			// 暂取空闲端口再释放：观测先启动时也不得占住业务将要绑定的端口。
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			address := ln.Addr().String()
			_, port, _ := net.SplitHostPort(address)
			number, err := strconv.Atoi(port)
			require.NoError(t, err)
			require.NoError(t, ln.Close())
			cfg := global.LifecycleConfig{Shutdown: time.Minute, Drain: time.Second}
			if business == "http" {
				cfg.Port = number
			} else {
				cfg.GRPC = global.GRPCConfig{Enabled: true, Address: address, Drain: time.Second, ConnectionTimeout: time.Second}
			}
			c := viper.New()
			c.Set("observability.metrics.enable", true)
			c.Set("observability.metrics.address", address)
			o := StartObservability(c, cfg)
			defer o.Shutdown(context.Background())
			require.Nil(t, o.monitor)
			require.Nil(t, o.Runtime.Registry)
			if business == "http" {
				server := MustStartHTTP(cfg, o.Runtime)
				defer server.Shutdown(context.Background())
				client := http.Client{Timeout: time.Second}
				resp, err := client.Get("http://" + address + "/healthz")
				require.NoError(t, err)
				require.Equal(t, 200, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
			} else {
				server := MustGrpcServ(cfg, o.Runtime)
				require.NoError(t, server.Shutdown(context.Background()))
			}
		})
	}
	require.False(t, conflictsBusinessPort("127.0.0.1:9091", global.LifecycleConfig{Port: 8080, GRPC: global.GRPCConfig{Enabled: true, Address: "127.0.0.1:9090"}}))
	require.False(t, conflictsBusinessPort("127.0.0.1:9090", global.LifecycleConfig{GRPC: global.GRPCConfig{Address: "127.0.0.1:9090"}}))
}

func TestMetricsListenDeadline(t *testing.T) {
	r, err := telemetry.New(context.Background(), telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: true}})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	s, err := listenMetrics(ctx, r, func(ctx context.Context, network, address string) (net.Listener, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	require.Nil(t, s)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestObservationShutdownPreservesProcessDeadline(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	o := startObservation(telemetry.Options{ServiceName: "test", Tracing: telemetry.TracingOptions{Enable: true, Endpoint: server.URL + "/v1/traces", SampleRatio: 1, ExportTimeout: time.Second, ShutdownTimeout: time.Second}}, telemetry.New)
	_, span := o.Runtime.Tracer().Start(context.Background(), "shutdown")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	require.NoError(t, o.Shutdown(ctx))
	require.NoError(t, ctx.Err(), "可选观测必须给日志关闭留下总退出预算")
}

func TestMonitorFailureDoesNotStopBusiness(t *testing.T) {
	o := startObservation(telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: true, Address: "127.0.0.1:0", ShutdownTimeout: time.Second}}, telemetry.New)
	server := MustStartHTTP(global.LifecycleConfig{Drain: time.Second}, o.Runtime)
	defer server.Shutdown(context.Background())
	defer o.Shutdown(context.Background())
	require.NoError(t, o.monitor.listener.Close())
	select {
	case <-o.watched:
	case <-time.After(time.Second):
		t.Fatal("监控失败未完成观察")
	}
	_, port, err := net.SplitHostPort(server.listener.Addr().String())
	require.NoError(t, err)
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	select {
	case <-server.Done():
		t.Fatal("业务服务随监控故障退出")
	default:
	}
	require.NoError(t, server.Shutdown(context.Background()))
	require.NoError(t, o.Shutdown(context.Background()))
}
