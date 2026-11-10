package telemetry

import (
	"context"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"serv_tpl/internal/requestctx"
	"serv_tpl/pkg/logger"
	versionv1 "serv_tpl/sdk/version/v1"
	"testing"
	"time"
)

type versionServer struct {
	versionv1.UnimplementedVersionServiceServer
	seen chan trace.SpanContext
}

func (s versionServer) Version(ctx context.Context, _ *versionv1.VersionRequest) (*versionv1.VersionReply, error) {
	s.seen <- trace.SpanContextFromContext(ctx)
	return &versionv1.VersionReply{}, nil
}

func TestGRPCPropagationAndFilter(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			r, exp := memoryRuntime(t, enabled)
			handler := r.NewServerStats()
			server := grpc.NewServer(grpc.StatsHandler(handler))
			seen := make(chan trace.SpanContext, 1)
			versionv1.RegisterVersionServiceServer(server, versionServer{seen: seen})
			healthpb.RegisterHealthServer(server, health.NewServer())
			handler.Register(server.GetServiceInfo())
			ln := bufconn.Listen(1024 * 1024)
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if recover() != nil {
						logger.L().Error("test grpc panic", zap.Stack("stack"))
					}
				}()
				_ = server.Serve(ln)
			}()
			defer func() { server.Stop(); _ = ln.Close(); <-done }()
			conn, err := grpc.NewClient("passthrough:///test", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()), r.GRPCClientOption())
			require.NoError(t, err)
			defer conn.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ctx = r.Propagator.Extract(ctx, propagation.MapCarrier{"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"})
			stale := metadata.Pairs("traceparent", "00-11111111111111111111111111111111-1111111111111111-01", "tracestate", "stale=secret")
			ctx = metadata.NewOutgoingContext(ctx, stale)
			_, err = versionv1.NewVersionServiceClient(conn).Version(ctx, &versionv1.VersionRequest{})
			require.NoError(t, err)
			remote := <-seen
			require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", remote.TraceID().String())
			require.Equal(t, !enabled, remote.IsRemote())
			require.Empty(t, remote.TraceState().String(), "旧 tracestate 不应进入新调用")
			require.Equal(t, []string{"stale=secret"}, stale.Get("tracestate"), "不得修改调用方 metadata")
			require.Equal(t, []string{"00-11111111111111111111111111111111-1111111111111111-01"}, stale.Get("traceparent"))
			require.Equal(t, codes.Unimplemented, status.Code(conn.Invoke(ctx, "/unknown/secret", &versionv1.VersionRequest{}, &versionv1.VersionReply{})))
			_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				families, _ := r.Registry.Gather()
				for _, f := range families {
					if f.GetName() == "service_grpc_requests_total" {
						return len(f.Metric) == 1 && f.Metric[0].Counter.GetValue() == 1
					}
				}
				return false
			}, time.Second, time.Millisecond)
			if enabled {
				require.Eventually(t, func() bool { return len(exp.GetSpans()) == 4 }, time.Second, time.Millisecond)
				var servers int
				var serverParent trace.SpanID
				for _, s := range exp.GetSpans() {
					if s.SpanKind == trace.SpanKindServer {
						servers++
						require.Equal(t, remote.SpanID(), s.SpanContext.SpanID())
						serverParent = s.Parent.SpanID()
					}
				}
				require.Equal(t, 1, servers)
				var matchedClient bool
				for _, s := range exp.GetSpans() {
					if s.SpanContext.SpanID() == serverParent {
						matchedClient = true
						require.Equal(t, trace.SpanKindClient, s.SpanKind)
						require.Equal(t, "00f067aa0ba902b7", s.Parent.SpanID().String())
					}
				}
				require.True(t, matchedClient, "服务端 span 必须直接继承客户端 span")
			} else {
				require.Equal(t, "00f067aa0ba902b7", remote.SpanID().String())
				require.Empty(t, exp.GetSpans())
			}
		})
	}
}

func TestGRPCStatsDecodeFailure(t *testing.T) {
	r, _ := memoryRuntime(t, false)
	s := r.NewServerStats()
	s.Register(map[string]grpc.ServiceInfo{"test.Service": {Methods: []grpc.MethodInfo{{Name: "Decode"}}}})
	ctx := s.TagRPC(context.Background(), &stats.RPCTagInfo{FullMethodName: "/test.Service/Decode"})
	require.Empty(t, requestctx.SpanID(ctx))
	s.HandleRPC(ctx, &stats.Begin{})
	s.HandleRPC(ctx, &stats.End{Error: status.Error(codes.Internal, "decode secret")})
	require.Equal(t, context.Background(), s.TagRPC(context.Background(), &stats.RPCTagInfo{FullMethodName: "/unknown/method"}))
	s.HandleRPC(context.Background(), &stats.End{})
	require.Equal(t, context.Background(), s.TagConn(context.Background(), &stats.ConnTagInfo{}))
	s.HandleConn(context.Background(), &stats.ConnBegin{})
	families, err := r.Registry.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == "service_grpc_inflight" {
			require.Zero(t, f.Metric[0].Gauge.GetValue())
		}
	}
}

type invalidProtoCodec struct{}

func (invalidProtoCodec) Name() string                { return "proto" }
func (invalidProtoCodec) Marshal(any) ([]byte, error) { return []byte{0xff}, nil }
func (invalidProtoCodec) Unmarshal([]byte, any) error { return nil }

func TestGRPCStreamAndWireDecodeFailure(t *testing.T) {
	r, exp := memoryRuntime(t, true)
	handler := r.NewServerStats()
	server := grpc.NewServer(grpc.StatsHandler(handler))
	desc := healthpb.Health_ServiceDesc
	desc.ServiceName = "business.Health"
	server.RegisterService(&desc, health.NewServer())
	handler.Register(server.GetServiceInfo())
	ln := bufconn.Listen(1024 * 1024)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recover() != nil {
				logger.L().Error("test grpc panic", zap.Stack("stack"))
			}
		}()
		_ = server.Serve(ln)
	}()
	defer func() { server.Stop(); _ = ln.Close(); <-done }()
	conn, err := grpc.NewClient("passthrough:///test", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return ln.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = conn.Invoke(ctx, "/business.Health/Check", &healthpb.HealthCheckRequest{}, &healthpb.HealthCheckResponse{}, grpc.ForceCodec(invalidProtoCodec{}))
	require.Equal(t, codes.Internal, status.Code(err))
	streamCtx, stop := context.WithCancel(ctx)
	defer stop()
	stream, err := conn.NewStream(streamCtx, &grpc.StreamDesc{ServerStreams: true}, "/business.Health/Watch")
	require.NoError(t, err)
	require.NoError(t, stream.SendMsg(&healthpb.HealthCheckRequest{}))
	require.NoError(t, stream.CloseSend())
	require.NoError(t, stream.RecvMsg(&healthpb.HealthCheckResponse{}))
	stop()
	require.Error(t, stream.RecvMsg(&healthpb.HealthCheckResponse{}))
	require.Eventually(t, func() bool { return len(exp.GetSpans()) == 2 }, time.Second, time.Millisecond)
	require.Eventually(t, func() bool {
		fs, _ := r.Registry.Gather()
		for _, f := range fs {
			if f.GetName() == "service_grpc_inflight" {
				return f.Metric[0].Gauge.GetValue() == 0
			}
		}
		return false
	}, time.Second, time.Millisecond)
}

func BenchmarkGRPCObservation(b *testing.B) {
	holder, err := logger.NewStdLogger("ERROR")
	require.NoError(b, err)
	logger.SetGlobal(holder)
	defer logger.SetGlobalLevel("INFO")
	for _, mode := range []string{"off", "unsampled", "sampled"} {
		b.Run(mode, func(b *testing.B) {
			enabled := mode != "off"
			r, _ := memoryRuntime(b, enabled)
			if !enabled {
				r.Metrics = nil
			}
			sampler := sdktrace.NeverSample()
			if mode == "sampled" {
				sampler = sdktrace.AlwaysSample()
			}
			provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithSyncer(tracetest.NewNoopExporter()))
			defer provider.Shutdown(context.Background())
			r.Provider = provider
			s := r.NewServerStats()
			s.Register(map[string]grpc.ServiceInfo{"bench": {Methods: []grpc.MethodInfo{{Name: "Call"}}}})
			b.ReportAllocs()
			for b.Loop() {
				ctx := s.TagRPC(context.Background(), &stats.RPCTagInfo{FullMethodName: "/bench/Call"})
				s.HandleRPC(ctx, &stats.Begin{})
				s.HandleRPC(ctx, &stats.End{})
			}
		})
	}
}
