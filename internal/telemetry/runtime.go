// Package telemetry 提供显式注入的指标、追踪和传播能力，不修改全局 OTel 实例。
package telemetry

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Options 是已校验的观测配置，配置解析归 global 所有。
type Options struct {
	ServiceName, Version, Environment string
	Metrics                           MetricsOptions
	Tracing                           TracingOptions
}

// MetricsOptions 定义独立监控端口与关闭期限。
type MetricsOptions struct {
	Enable          bool
	Address         string
	ShutdownTimeout time.Duration
}

// TracingOptions 定义 OTLP/HTTP 导出和根链路采样。
type TracingOptions struct {
	Enable                         bool
	Endpoint                       string
	SampleRatio                    float64
	ExportTimeout, ShutdownTimeout time.Duration
}

// Runtime 持有本进程的独立 Registry 和 Provider，由启动层负责关闭。
type Runtime struct {
	Options    Options
	Registry   *prometheus.Registry
	Metrics    *Metrics
	Provider   trace.TracerProvider
	Propagator propagation.TextMapPropagator
	provider   *sdktrace.TracerProvider
	processor  sdktrace.SpanProcessor
	transport  *http.Transport
	once       sync.Once
	err        error
}

// New 创建观测实例；不等待 Collector 可用，也不启动监听器。
func New(ctx context.Context, o Options) (*Runtime, error) {
	r := &Runtime{Options: o, Provider: noop.NewTracerProvider(), Propagator: propagation.TraceContext{}}
	if o.Metrics.Enable {
		r.Registry = prometheus.NewRegistry()
		r.Metrics = newMetrics(r.Registry)
		r.Registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	}
	if !o.Tracing.Enable {
		return r, nil
	}
	// 步骤1：专用网络客户端不埋点，显式限制连接和总请求时间。
	r.transport = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: o.Tracing.ExportTimeout, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: o.Tracing.ExportTimeout, ResponseHeaderTimeout: o.Tracing.ExportTimeout,
		MaxIdleConns: 2, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(o.Tracing.Endpoint),
		otlptracehttp.WithHeaders(map[string]string{}),
		otlptracehttp.WithCompression(otlptracehttp.NoCompression),
		otlptracehttp.WithTimeout(o.Tracing.ExportTimeout),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{
			Enabled: true, InitialInterval: 100 * time.Millisecond,
			MaxInterval: time.Second, MaxElapsedTime: o.Tracing.ExportTimeout,
		}),
		otlptracehttp.WithHTTPClient(&http.Client{
			Transport: r.transport, Timeout: o.Tracing.ExportTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}),
	)
	if err != nil {
		r.transport.CloseIdleConnections()
		return nil, err
	}
	// 步骤2：SDK WithResource 会合并环境属性，出口视图固定使用本地 Resource。
	// 所有队列和 span 限额均显式指定。
	res := resource.NewSchemaless(
		attribute.String("service.name", o.ServiceName), attribute.String("service.instance.id", uuid.NewString()),
		attribute.String("service.version", o.Version), attribute.String("deployment.environment.name", o.Environment),
	)
	r.processor = sdktrace.NewBatchSpanProcessor(&safeExporter{next: exporter, resource: res},
		sdktrace.WithMaxQueueSize(2048), sdktrace.WithMaxExportBatchSize(512),
		sdktrace.WithBatchTimeout(time.Second), sdktrace.WithExportTimeout(o.Tracing.ExportTimeout))
	r.provider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(o.Tracing.SampleRatio))),
		sdktrace.WithRawSpanLimits(sdktrace.SpanLimits{
			AttributeValueLengthLimit: 256, AttributeCountLimit: 32, EventCountLimit: 16,
			LinkCountLimit: 8, AttributePerEventCountLimit: 8, AttributePerLinkCountLimit: 8,
		}),
		sdktrace.WithSpanProcessor(r.processor),
	)
	r.Provider = r.provider
	return r, nil
}

// Tracer 返回供业务显式注入的 tracer。
func (r *Runtime) Tracer() trace.Tracer { return r.Provider.Tracer("serv_tpl") }

// Handler 仅提供 /metrics，禁止在监控端口挂业务或健康路由。
func (r *Runtime) Handler() http.Handler {
	if r.Registry == nil {
		return http.NotFoundHandler()
	}
	metrics := promhttp.HandlerFor(r.Registry, promhttp.HandlerOpts{MaxRequestsInFlight: 5, Timeout: 5 * time.Second})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/metrics" {
			http.NotFound(w, req)
			return
		}
		metrics.ServeHTTP(w, req)
	})
}

// Shutdown 幂等关闭并刷新剩余 span，期限独立于业务请求。
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.once.Do(func() {
		if r.provider != nil {
			stage, cancel := context.WithTimeout(ctx, r.Options.Tracing.ShutdownTimeout)
			defer cancel()
			r.err = r.provider.Shutdown(stage)
			// SDK 在已取消的 ctx 下可能尚未通知处理器就返回；显式通知停止，
			// 不增加预算。BatchSpanProcessor.Shutdown 自身幂等，不重复 flush。
			if r.err != nil && r.processor != nil {
				r.err = errors.Join(r.err, r.processor.Shutdown(stage))
			}
			r.transport.CloseIdleConnections()
		}
	})
	return r.err
}
