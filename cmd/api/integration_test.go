package main

import (
	"context"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestEntryIsolated 运行完整 main 子进程；-short 跳过，非短模式仍须显式提供隔离 MySQL。
// Redis 使用本测试创建的 miniredis，不连接外部 Redis。
func TestEntryIsolated(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过完整入口启动/退出集成；需在非短模式显式配置 D02_TEST_MYSQL_DSN")
	}
	dsn := os.Getenv("D02_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("需要隔离 MySQL 的 D02_TEST_MYSQL_DSN")
	}
	for _, mode := range []string{"http-only", "http-and-grpc", "http-conflict", "grpc-conflict"} {
		t.Run(mode, func(t *testing.T) {
			occupied := mode == "http-conflict" || mode == "grpc-conflict"
			if !occupied && runtime.GOOS == "windows" {
				t.Skip("正常 SIGTERM 退出在 Linux 验证")
			}
			redis := miniredis.RunT(t)
			ln, err := net.Listen("tcp", ":0")
			require.NoError(t, err)
			defer ln.Close()
			port := ln.Addr().(*net.TCPAddr).Port
			if mode != "http-conflict" {
				require.NoError(t, ln.Close())
			}
			grpcLn, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer grpcLn.Close()
			grpcAddr := grpcLn.Addr().String()
			if mode != "grpc-conflict" {
				require.NoError(t, grpcLn.Close())
			}
			grpcEnabled := mode == "http-and-grpc" || mode == "grpc-conflict"
			cmd, ctx := entryCommand(t, fmt.Sprintf("mysql:\n  dsn: '%s'\nredis:\n  addr: '%s'\nhttp:\n  port: %d\ngrpc:\n  enable: %t\n  address: '%s'\nmetrics:\n  enable: true\n  dt: 1\n", dsn, redis.Addr(), port, grpcEnabled, grpcAddr))
			if occupied {
				out, err := cmd.CombinedOutput()
				require.NoError(t, ctx.Err())
				var exited *exec.ExitError
				require.ErrorAs(t, err, &exited)
				require.NotZero(t, exited.ExitCode())
				want := "panic: HTTP 监听失败"
				if mode == "grpc-conflict" {
					want = "panic: gRPC 监听失败"
				}
				require.Contains(t, string(out), want)
				return
			}
			require.NoError(t, cmd.Start())
			joined := false
			defer func() {
				if !joined {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			client := &http.Client{Timeout: time.Second}
			defer client.CloseIdleConnections()
			require.Eventually(t, func() bool {
				resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/ready", port))
				if err != nil {
					return false
				}
				defer resp.Body.Close()
				return resp.StatusCode == http.StatusOK
			}, 5*time.Second, 10*time.Millisecond)
			if grpcEnabled {
				conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				require.NoError(t, err)
				defer conn.Close()
				rpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				resp, err := healthpb.NewHealthClient(conn).Check(rpcCtx, &healthpb.HealthCheckRequest{}, grpc.WaitForReady(true))
				require.NoError(t, err)
				require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
			}
			require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
			err = cmd.Wait()
			joined = true
			require.NoError(t, ctx.Err())
			require.NoError(t, err, "正常运行后 SIGTERM 应清理完成并退出 0")
		})
	}
}
