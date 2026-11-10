package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"runtime"
	"serv_tpl/internal/global"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestProcessHelper 仅由本文件子进程测试启动，不接触存储。
func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("D02_PROCESS_HELPER")
	if mode == "" {
		return
	}
	code := processRun(mode)
	os.Exit(code)
}

func processRun(mode string) int {
	if mode == "optional-worker" {
		global.AppConf = viper.New()
		global.AppConf.Set("shutdown_timeout", 2)
		global.AppConf.Set("metrics.enable", "invalid")
		global.AppConf.Set("observability.metrics.enable", true)
		global.AppConf.Set("observability.metrics.address", "invalid")
		go announceWaitingExit()
		cfg := global.MustLoadWorkerLifecycleConfig(global.AppConf)
		logSync := MustInitLog()
		observation := StartObservability(global.AppConf, cfg)
		return WaitingWorkerExit(cfg.Shutdown, nil, nil, Cleanup{Name: "observability", Stop: observation.Shutdown}, Cleanup{Name: "log", Stop: logSync})
	}
	if mode == "panic" {
		panic("startup")
	}
	httpDone := make(chan error, 1)
	if mode == "error" || mode == "http-hang" || mode == "http-log-hang" {
		httpDone <- errors.New("HTTP serve failure")
	} else {
		httpDone = nil // 无服务通道时仍须处理信号，覆盖 worker 入口的等待方式。
		// 仅测试观察：确认公开入口已阻塞等待事件后才通知父进程发信号。
		// 不在生产函数中增加就绪回调或可替换的全局变量。
		go announceWaitingExit()
	}
	timeout := 200 * time.Millisecond
	if mode == "second" {
		timeout = 10 * time.Second // 父进程期限更短，防止总超时冒充二次信号通过。
	}
	return WaitingExit(timeout, httpDone, nil, nil,
		Cleanup{Name: "http", Stop: func(context.Context) error {
			fmt.Println("DRAINING")
			if mode == "hang" || mode == "second" || mode == "http-hang" {
				select {}
			}
			return nil
		}},
		Cleanup{Name: "storage", Stop: func(context.Context) error { fmt.Println("STORAGE_CLOSED"); return nil }},
		Cleanup{Name: "log", Stop: func(context.Context) error {
			if mode == "log-hang" || mode == "http-log-hang" {
				fmt.Println("LOG_CLOSING")
				select {}
			}
			fmt.Println("LOG_CLOSED")
			return nil
		}},
	)
}

// announceWaitingExit 只用于独立子进程；栈格式不匹配时明确超时失败，不盲发信号。
func announceWaitingExit() {
	defer func() {
		if v := recover(); v != nil {
			fmt.Fprintln(os.Stderr, "等待退出入口就绪时 panic:", v)
			os.Exit(2)
		}
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(stack, " [select]:") && (strings.Contains(stack, "/internal/bootstrap.WaitingExit(") || strings.Contains(stack, "/internal/bootstrap.WaitingWorkerExit(")) {
				fmt.Println("READY")
				return
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			fmt.Fprintln(os.Stderr, "未观察到 WaitingExit 等待事件")
			os.Exit(2)
		}
	}
}

func TestProcessTimeout(t *testing.T) {
	for _, mode := range []string{"http-hang", "http-log-hang"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$")
			cmd.Env = append(os.Environ(), "D02_PROCESS_HELPER="+mode)
			out, err := cmd.CombinedOutput()
			require.NoError(t, ctx.Err(), "必须由退出看门狗结束，不能由测试期限杀死")
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit)
			require.Equal(t, 1, exit.ExitCode())
			require.Contains(t, string(out), "DRAINING")
			if mode == "http-log-hang" {
				require.Contains(t, string(out), "LOG_CLOSING")
			}
			require.NotContains(t, string(out), "LOG_CLOSED")
		})
	}
}
func TestProcessFailureExit(t *testing.T) {
	for _, mode := range []string{"error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$")
			cmd.Env = append(os.Environ(), "D02_PROCESS_HELPER="+mode)
			out, e := cmd.CombinedOutput()
			var exit *exec.ExitError
			require.ErrorAs(t, e, &exit)
			if mode == "panic" {
				require.NotZero(t, exit.ExitCode())
				require.Contains(t, string(out), "panic: startup")
				require.NotContains(t, string(out), "LOG_CLOSED")
				return
			}
			require.Equal(t, 1, exit.ExitCode())
			require.Contains(t, string(out), "DRAINING")
			require.Contains(t, string(out), "LOG_CLOSED")
		})
	}
}
func TestProcessSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux 信号验证，不以 Windows 模拟代替")
	}
	for _, tc := range []struct {
		name, mode string
		signal     os.Signal
		second     bool
		code       int
	}{
		{"term", "normal", syscall.SIGTERM, false, 0}, {"int", "normal", os.Interrupt, false, 0},
		{"timeout", "hang", syscall.SIGTERM, false, 1}, {"second", "second", syscall.SIGTERM, true, 1},
		{"log-timeout", "log-hang", syscall.SIGTERM, false, 1},
		{"optional-worker", "optional-worker", syscall.SIGTERM, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$")
			cmd.Env = append(os.Environ(), "D02_PROCESS_HELPER="+tc.mode)
			pipe, e := cmd.StdoutPipe()
			require.NoError(t, e)
			require.NoError(t, cmd.Start())
			defer func() { _ = cmd.Process.Kill() }()
			scanner := bufio.NewScanner(pipe)
			ready := false
			for scanner.Scan() {
				if scanner.Text() == "READY" {
					ready = true
					break
				}
			}
			require.True(t, ready, "子进程未进入 WaitingExit 等待状态")
			require.NoError(t, cmd.Process.Signal(tc.signal))
			var lines []string
			for scanner.Scan() {
				line := scanner.Text()
				lines = append(lines, line)
				if line == "DRAINING" && tc.second {
					require.NoError(t, cmd.Process.Signal(tc.signal))
				}
			}
			e = cmd.Wait()
			require.NoError(t, ctx.Err())
			if tc.code == 0 {
				require.NoError(t, e)
				if tc.mode == "optional-worker" {
					return
				}
				require.Contains(t, strings.Join(lines, " "), "LOG_CLOSED")
				require.Equal(t, []string{"DRAINING", "STORAGE_CLOSED", "LOG_CLOSED"}, lines)
			} else {
				var exit *exec.ExitError
				require.ErrorAs(t, e, &exit)
				require.Equal(t, tc.code, exit.ExitCode())
			}
		})
	}
}
