package main

import (
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
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
	for _, occupied := range []bool{false, true} {
		t.Run(fmt.Sprint(occupied), func(t *testing.T) {
			if !occupied && runtime.GOOS == "windows" {
				t.Skip("正常 SIGTERM 退出在 Linux 验证")
			}
			redis := miniredis.RunT(t)
			ln, err := net.Listen("tcp", ":0")
			require.NoError(t, err)
			defer ln.Close()
			port := ln.Addr().(*net.TCPAddr).Port
			if !occupied {
				require.NoError(t, ln.Close())
			}
			cmd, ctx := entryCommand(t, fmt.Sprintf("mysql:\n  dsn: '%s'\nredis:\n  addr: '%s'\nhttp:\n  port: %d\nmetrics:\n  enable: true\n  dt: 1\n", dsn, redis.Addr(), port))
			if occupied {
				out, err := cmd.CombinedOutput()
				require.NoError(t, ctx.Err())
				var exited *exec.ExitError
				require.ErrorAs(t, err, &exited)
				require.NotZero(t, exited.ExitCode())
				require.Contains(t, string(out), "panic: HTTP 监听失败")
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
			require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
			err = cmd.Wait()
			joined = true
			require.NoError(t, ctx.Err())
			require.NoError(t, err, "正常运行后 SIGTERM 应清理完成并退出 0")
		})
	}
}
