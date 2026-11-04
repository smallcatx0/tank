package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"runtime"
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
	code := Run(func(ctx context.Context, r *Runtime) error {
		r.SetShutdownTimeout(200 * time.Millisecond)
		r.Add("log", func(context.Context) error { fmt.Println("LOG_CLOSED"); return nil })
		r.Add("http", func(context.Context) error {
			fmt.Println("DRAINING")
			if mode == "hang" {
				select {}
			}
			return nil
		})
		go func() {
			defer func() {
				if recover() != nil {
					r.Fail()
					logPanic("process-test")
				}
			}()
			for !r.Ready() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Millisecond):
				}
			}
			fmt.Println("READY")
		}()
		if mode == "error" {
			return errors.New("failure")
		}
		return nil
	})
	os.Exit(code)
}
func TestProcessFailureExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), "D02_PROCESS_HELPER=error")
	out, e := cmd.CombinedOutput()
	var exit *exec.ExitError
	require.ErrorAs(t, e, &exit)
	require.Equal(t, 1, exit.ExitCode())
	require.Contains(t, string(out), "DRAINING")
	require.Contains(t, string(out), "LOG_CLOSED")
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
		{"timeout", "hang", syscall.SIGTERM, false, 1}, {"second", "hang", syscall.SIGTERM, true, 1},
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
			for scanner.Scan() {
				if scanner.Text() == "READY" {
					break
				}
			}
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
			if tc.code == 0 {
				require.NoError(t, e)
				require.Contains(t, strings.Join(lines, " "), "LOG_CLOSED")
			} else {
				var exit *exec.ExitError
				require.ErrorAs(t, e, &exit)
				require.Equal(t, tc.code, exit.ExitCode())
			}
		})
	}
}
