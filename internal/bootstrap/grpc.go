package bootstrap

import (
	"context"
	"errors"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"serv_tpl/internal/global"
	"serv_tpl/internal/requestctx"
	"serv_tpl/internal/telemetry"
	"serv_tpl/pkg/logger"
	"serv_tpl/routes"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// GRPCService 持有 gRPC 服务与退出结果；nil 表示配置未开启。
type GRPCService struct {
	server        *grpc.Server
	health        *health.Server
	listener      net.Listener
	done          chan error
	finished      chan struct{}
	stopping      atomic.Bool
	once          sync.Once
	err, serveErr error
	drain         time.Duration
}

// MustGrpcServ 按开关装配 gRPC，启用时同步监听，启动失败直接 panic。
// observation 必须非 nil；关闭观测时传入关闭态 Runtime。
func MustGrpcServ(cfg global.LifecycleConfig, observation *telemetry.Runtime) *GRPCService {
	if !cfg.GRPC.Enabled {
		return nil
	}
	obs := observation.NewServerStats()
	opts := []grpc.ServerOption{grpc.ConnectionTimeout(cfg.GRPC.ConnectionTimeout), grpc.UnaryInterceptor(recoverUnaryRPC), grpc.StreamInterceptor(recoverStreamRPC), grpc.StatsHandler(obs)}
	// 步骤1：在监听前装配拦截器、生命周期健康服务和业务注册入口。
	s := &GRPCService{
		server: grpc.NewServer(opts...),
		health: health.NewServer(), done: make(chan error, 1), finished: make(chan struct{}),
		drain: cfg.GRPC.Drain,
	}
	healthpb.RegisterHealthServer(s.server, s.health)
	routes.RegisterGRPC(s.server)
	obs.Register(s.server.GetServiceInfo())
	ln, err := net.Listen("tcp", cfg.GRPC.Address)
	if err != nil {
		panic("gRPC 监听失败")
	}
	s.listener = ln
	// 步骤2：Serve 故障交给 WaitingExit，panic 不泄漏请求或敏感值。
	go func() {
		var serveErr error
		defer close(s.finished)
		defer func() {
			panicked := recover() != nil
			if panicked {
				serveErr = errors.New("gRPC Serve panic")
			}
			if serveErr == nil && !s.stopping.Load() {
				serveErr = errors.New("gRPC 意外停止")
			}
			s.serveErr = serveErr
			s.done <- serveErr
			close(s.done)
			if panicked {
				logPanic("grpc")
			}
		}()
		serveErr = s.server.Serve(ln)
		if s.stopping.Load() && errors.Is(serveErr, grpc.ErrServerStopped) {
			serveErr = nil
		}
	}()
	return s
}

// Done 返回服务结果；禁用时返回 nil，不参与退出事件等待。
func (s *GRPCService) Done() <-chan error {
	if s == nil {
		return nil
	}
	return s.done
}

// Shutdown 幂等排空 RPC；超时强制断连，忽略取消的业务处理器由进程退出兜底。
func (s *GRPCService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		s.stopping.Store(true)
		s.health.Shutdown()
		drain, cancel := context.WithTimeout(ctx, s.drain)
		defer cancel()
		drained := make(chan error, 1)
		go func() {
			var err error
			defer func() {
				if recover() != nil {
					err = errors.New("gRPC GracefulStop panic")
					logPanic("grpc-shutdown")
				}
				drained <- err
			}()
			s.server.GracefulStop()
		}()
		select {
		case s.err = <-drained:
		case <-drain.Done():
			s.err = drain.Err()
		}
		if s.err != nil {
			s.server.Stop()
		}
		select {
		case <-s.finished:
			s.err = errors.Join(s.err, s.serveErr)
		case <-ctx.Done():
			s.err = errors.Join(s.err, ctx.Err())
		}
	})
	return s.err
}

func recoverUnaryRPC(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (reply any, err error) {
	defer func() {
		if recover() != nil {
			logger.L().Error("gRPC handler panic", zap.String("method", info.FullMethod), zap.String("trace_id", requestctx.TraceID(ctx)), zap.String("span_id", requestctx.SpanID(ctx)), zap.ByteString("stack", debug.Stack()))
			reply, err = nil, status.Error(codes.Internal, "内部服务错误")
		}
	}()
	return handler(ctx, req)
}

func recoverStreamRPC(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	defer func() {
		if recover() != nil {
			logger.L().Error("gRPC stream panic", zap.String("method", info.FullMethod), zap.String("trace_id", requestctx.TraceID(stream.Context())), zap.String("span_id", requestctx.SpanID(stream.Context())), zap.ByteString("stack", debug.Stack()))
			err = status.Error(codes.Internal, "内部服务错误")
		}
	}()
	return handler(srv, stream)
}
