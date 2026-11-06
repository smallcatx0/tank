package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	versionv1 "serv_tpl/sdk/version/v1"
	"serv_tpl/internal/global"
	"serv_tpl/routes"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func startTestGRPC(t *testing.T, drain time.Duration) (*GRPCService, healthpb.HealthClient) {
	t.Helper()
	s := MustGrpcServ(global.LifecycleConfig{GRPC: global.GRPCConfig{
		Enabled: true, Address: "127.0.0.1:0", Drain: drain, ConnectionTimeout: time.Second,
	}})
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	conn, err := grpc.NewClient(s.listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return s, healthpb.NewHealthClient(conn)
}

func TestGRPCVersionMatchesHTTP(t *testing.T) {
	previous := global.AppConf
	t.Cleanup(func() { global.AppConf = previous })
	global.AppConf = viper.New()
	global.AppConf.Set("name", "app")
	global.AppConf.Set("desc", "应用程序配置")
	global.AppConf.Set("ver", "1.2.3")
	global.AppConf.Set("version", "不得使用此键")
	s, _ := startTestGRPC(t, time.Second)
	conn, err := grpc.NewClient(s.listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := versionv1.NewVersionServiceClient(conn)
	resp, err := client.Version(ctx, &versionv1.VersionRequest{})
	require.NoError(t, err)
	engine := gin.New()
	routes.RegisterHTTP(engine, func() bool { return true })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var httpInfo map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &httpInfo))
	require.Equal(t, httpInfo, map[string]string{
		"name": resp.GetName(), "desc": resp.GetDesc(), "version": resp.GetVersion(),
	})
}

func TestGRPCDisabledAndListenFailure(t *testing.T) {
	s := MustGrpcServ(global.LifecycleConfig{})
	require.Nil(t, s)
	require.Nil(t, s.Done())
	require.NoError(t, s.Shutdown(context.Background()))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	require.PanicsWithValue(t, "gRPC 监听失败", func() {
		MustGrpcServ(global.LifecycleConfig{GRPC: global.GRPCConfig{Enabled: true, Address: ln.Addr().String()}})
	})
}

func TestGRPCHealthAndShutdown(t *testing.T) {
	s, client := startTestGRPC(t, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, result.Status)
	require.NoError(t, s.Shutdown(ctx))
	require.NoError(t, s.Shutdown(ctx))
	require.NoError(t, <-s.Done())
	_, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.Error(t, err)
}

func TestGRPCDrain(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "graceful", true: "timeout"}[timeout], func(t *testing.T) {
			drain := 2 * time.Second
			if timeout {
				drain = 100 * time.Millisecond
			}
			s, client := startTestGRPC(t, drain)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			watchCtx, stopWatch := context.WithCancel(ctx)
			defer stopWatch()
			watch, err := client.Watch(watchCtx, &healthpb.HealthCheckRequest{})
			require.NoError(t, err)
			_, err = watch.Recv()
			require.NoError(t, err)
			done := make(chan error, 1)
			async(t, func() { done <- s.Shutdown(ctx) })
			result, err := watch.Recv()
			require.NoError(t, err)
			require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, result.Status)
			if !timeout {
				select {
				case err := <-done:
					t.Fatalf("活跃 RPC 尚未结束却退出：%v", err)
				default:
				}
				stopWatch()
			}
			select {
			case err := <-done:
				if timeout {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				} else {
					require.NoError(t, err)
				}
			case <-ctx.Done():
				t.Fatal("排空未在期限内结束")
			}
			_, err = watch.Recv()
			require.Error(t, err)
		})
	}
}

func TestGRPCServeFailure(t *testing.T) {
	s, client := startTestGRPC(t, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.NoError(t, s.listener.Close())
	select {
	case err := <-s.Done():
		require.Error(t, err)
	case <-ctx.Done():
		t.Fatal("Serve 未通知故障")
	}
	require.Error(t, s.Shutdown(ctx), "信号与故障并发时也不得丢失 Serve 错误")
}

func TestGRPCRecovery(t *testing.T) {
	reply, err := recoverUnaryRPC(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test/check"}, func(context.Context, any) (any, error) { panic("secret") })
	require.Nil(t, reply)
	require.Equal(t, codes.Internal, status.Code(err))
	require.NotContains(t, err.Error(), "secret")
	err = recoverStreamRPC(nil, nil, &grpc.StreamServerInfo{FullMethod: "/test/watch"}, func(any, grpc.ServerStream) error { panic("secret") })
	require.Equal(t, codes.Internal, status.Code(err))
	require.NotContains(t, err.Error(), "secret")
	want := errors.New("business error")
	_, err = recoverUnaryRPC(context.Background(), nil, &grpc.UnaryServerInfo{}, func(context.Context, any) (any, error) { return nil, want })
	require.ErrorIs(t, err, want)
	err = recoverStreamRPC(nil, nil, &grpc.StreamServerInfo{}, func(any, grpc.ServerStream) error { return want })
	require.ErrorIs(t, err, want)
}
