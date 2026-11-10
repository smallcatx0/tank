package telemetry

import (
	"context"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"serv_tpl/internal/requestctx"
	"serv_tpl/pkg/logger"
	"strings"
	"time"
)

type rpcKey struct{}
type rpcInfo struct {
	service, method string
	started         time.Time
}
type metadataCarrier metadata.MD

func (m metadataCarrier) Get(k string) string {
	v := metadata.MD(m).Get(k)
	if len(v) != 1 {
		return ""
	}
	return v[0]
}
func (m metadataCarrier) Set(k, v string) { metadata.MD(m).Set(k, v) }
func (m metadataCarrier) Keys() []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ServerStats 组合追踪和固定指标，只允许注册过的业务方法。
// Register 必须在 Serve 之前调用，之后集合只读。
type ServerStats struct {
	runtime *Runtime
	tracing stats.Handler
	methods map[string]rpcInfo
}

// NewServerStats 构造服务端观测，未启用追踪时只提取上下文。
func (r *Runtime) NewServerStats() *ServerStats {
	s := &ServerStats{runtime: r, methods: map[string]rpcInfo{}}
	if r.Options.Tracing.Enable {
		s.tracing = otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(r.Provider), otelgrpc.WithPropagators(r.Propagator), otelgrpc.WithMeterProvider(metricnoop.NewMeterProvider()))
	}
	return s
}

// Register 冻结 gRPC 已注册方法白名单，排除 health 和未知方法。
func (s *ServerStats) Register(info map[string]grpc.ServiceInfo) {
	for service, v := range info {
		if service == "grpc.health.v1.Health" {
			continue
		}
		for _, m := range v.Methods {
			s.methods["/"+service+"/"+m.Name] = rpcInfo{service: service, method: m.Name}
		}
	}
}

// TagRPC 先创建官方 span 再附加计时上下文，未知方法不创建 span。
func (s *ServerStats) TagRPC(ctx context.Context, i *stats.RPCTagInfo) context.Context {
	info, ok := s.methods[i.FullMethodName]
	if !ok {
		return ctx
	}
	if s.tracing != nil {
		ctx = s.tracing.TagRPC(ctx, i)
	} else {
		md, _ := metadata.FromIncomingContext(ctx)
		ctx = s.runtime.Propagator.Extract(ctx, metadataCarrier(md))
	}
	info.started = time.Now()
	ctx = requestctx.WithTrace(ctx, "")
	return context.WithValue(ctx, rpcKey{}, info)
}

// HandleRPC 按 Begin/End 统计完整 RPC，包括处理前解码失败。
func (s *ServerStats) HandleRPC(ctx context.Context, event stats.RPCStats) {
	info, ok := ctx.Value(rpcKey{}).(rpcInfo)
	if !ok {
		return
	}
	if s.tracing != nil {
		s.tracing.HandleRPC(ctx, event)
	}
	m := s.runtime.Metrics
	switch e := event.(type) {
	case *stats.Begin:
		if m != nil {
			m.grpcInflight.Inc()
		}
	case *stats.End:
		code := status.Code(e.Error).String()
		if m != nil {
			m.grpcInflight.Dec()
			m.grpcCount.WithLabelValues(info.service, info.method, code).Inc()
			m.grpcDuration.WithLabelValues(info.service, info.method).Observe(time.Since(info.started).Seconds())
		}
		logger.L().Info("gRPC 请求完成", zap.String("service", info.service), zap.String("method", info.method), zap.String("code", code), zap.String("trace_id", requestctx.TraceID(ctx)), zap.String("span_id", requestctx.SpanID(ctx)), zap.Float64("latency_ms", requestctx.LatencyMS(ctx)))
	}
}

// TagConn 转发连接上下文。
func (s *ServerStats) TagConn(ctx context.Context, i *stats.ConnTagInfo) context.Context {
	if s.tracing != nil {
		return s.tracing.TagConn(ctx, i)
	}
	return ctx
}

// HandleConn 转发连接事件。
func (s *ServerStats) HandleConn(ctx context.Context, e stats.ConnStats) {
	if s.tracing != nil {
		s.tracing.HandleConn(ctx, e)
	}
}

// clientStats 清理旧传播字段，并按开关转发给官方追踪 handler。
type clientStats struct {
	runtime *Runtime
	tracing stats.Handler
}

func (s clientStats) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	for _, k := range s.runtime.Propagator.Fields() {
		delete(md, strings.ToLower(k))
	}
	s.runtime.Propagator.Inject(ctx, metadataCarrier(md))
	ctx = metadata.NewOutgoingContext(ctx, md)
	if s.tracing != nil {
		return s.tracing.TagRPC(ctx, info)
	}
	return ctx
}
func (s clientStats) HandleRPC(ctx context.Context, event stats.RPCStats) {
	if s.tracing != nil {
		s.tracing.HandleRPC(ctx, event)
	}
}
func (s clientStats) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	if s.tracing != nil {
		return s.tracing.TagConn(ctx, info)
	}
	return ctx
}
func (s clientStats) HandleConn(ctx context.Context, event stats.ConnStats) {
	if s.tracing != nil {
		s.tracing.HandleConn(ctx, event)
	}
}

// GRPCClientOption 只装配传播与追踪，连接地址、复用和调用超时仍由调用者负责。
func (r *Runtime) GRPCClientOption() grpc.DialOption {
	s := clientStats{runtime: r}
	if r.Options.Tracing.Enable {
		s.tracing = otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(r.Provider), otelgrpc.WithPropagators(r.Propagator), otelgrpc.WithMeterProvider(metricnoop.NewMeterProvider()))
	}
	return grpc.WithStatsHandler(s)
}
