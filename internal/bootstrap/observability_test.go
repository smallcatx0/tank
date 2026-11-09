package bootstrap

import (
	"context"
	versionv1 "gitee.com/chengduzhihuige/go-serv-template/sdk/version/v1"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"net/http"
	"net/http/httptest"
	"serv_tpl/internal/global"
	"serv_tpl/internal/httpmd"
	"serv_tpl/internal/requestctx"
	"serv_tpl/internal/telemetry"
	"serv_tpl/pkg/kafka"
	"testing"
	"time"
)

func TestHTTPToGRPCObservation(t *testing.T) {
	r, err := telemetry.New(context.Background(), telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: true}})
	require.NoError(t, err)
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer provider.Shutdown(context.Background())
	r.Provider = provider
	r.Options.Tracing.Enable = true
	cfg := global.LifecycleConfig{GRPC: global.GRPCConfig{Enabled: true, Address: "127.0.0.1:0", ConnectionTimeout: time.Second, Drain: time.Second}}
	rpc := MustGrpcServ(cfg, r)
	defer rpc.Shutdown(context.Background())
	conn, err := grpc.NewClient(rpc.listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), r.GRPCClientOption())
	require.NoError(t, err)
	defer conn.Close()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpmd.Observe(r), httpmd.ReqLog, httpmd.Recovery)
	engine.GET("/call", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		_, err := versionv1.NewVersionServiceClient(conn).Version(ctx, &versionv1.VersionRequest{})
		if err != nil {
			c.Status(502)
			return
		}
		c.String(200, requestctx.TraceID(ctx))
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: r.HTTPTransport(transport), Timeout: 2 * time.Second}
	resp, err := client.Get(server.URL + "/call")
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, string(data), resp.Header.Get("x-b3-traceid"))
	require.Eventually(t, func() bool { return len(exporter.GetSpans()) == 4 }, time.Second, time.Millisecond)
	spans := exporter.GetSpans()
	ids := map[trace.SpanID]bool{}
	roots := 0
	for _, s := range spans {
		require.Equal(t, string(data), s.SpanContext.TraceID().String())
		ids[s.SpanContext.SpanID()] = true
		if !s.Parent.IsValid() {
			roots++
		}
	}
	require.Len(t, ids, 4)
	require.Equal(t, 1, roots)
	for _, s := range spans {
		if s.Parent.IsValid() {
			require.True(t, ids[s.Parent.SpanID()])
		}
	}
	// 实际 API 装配也可注入观测，健康路由不额外创建 span。
	api := MustStartHTTP(global.LifecycleConfig{Drain: time.Second}, r)
	require.NoError(t, api.Shutdown(context.Background()))
}

func TestKafkaCollectorRegistrationFailure(t *testing.T) {
	o := kafka.DefaultOptions()
	o.Brokers = []string{"127.0.0.1:1"}
	o.Topics = []string{"test"}
	o.GroupID = "test"
	o.StartOffset = "earliest"
	r, err := telemetry.New(context.Background(), telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: true}})
	require.NoError(t, err)
	existing, err := kafka.NewConsumer(o)
	require.NoError(t, err)
	defer existing.Shutdown(context.Background())
	require.NoError(t, r.Registry.Register(existing))
	// 重复注册不能阻止新消费者构造，也不能覆盖已有 Collector。
	old := global.KafkaCli
	defer func() { global.KafkaCli = old }()
	s := MustKafkaWorker(global.KafkaConfig{Enabled: true, Options: o, Drain: time.Second}, func(context.Context, kafka.Message) error { return nil }, r)
	require.NotNil(t, s)
	require.NotNil(t, global.KafkaCli)
	require.NoError(t, s.Shutdown(context.Background()))
	require.True(t, r.Registry.Unregister(existing))
}

// closedRuntime 显式提供关闭态观测，测试沿用实际入口的装配路径。
func closedRuntime(t *testing.T) *telemetry.Runtime {
	t.Helper()
	r, err := telemetry.New(context.Background(), telemetry.Options{})
	require.NoError(t, err)
	return r
}
