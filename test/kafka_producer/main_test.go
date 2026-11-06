package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigDefaultsAndPriority(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.dev")
	require.NoError(t, os.WriteFile(path, []byte("KAFKA_BROKERS=localhost:9092\nKAFKA_TOPIC=file-topic\nDB_PASSWORD=private\n"), 0600))
	empty := func(string) string { return "" }
	cfg, err := parseConfig([]string{"-env-file", path}, io.Discard, empty)
	require.NoError(t, err)
	require.Equal(t, []string{"localhost:9092"}, cfg.brokers)
	require.Equal(t, "file-topic", cfg.topic)
	require.Equal(t, 1, cfg.count)
	require.Equal(t, 15*time.Second, cfg.timeout)
	require.Equal(t, "hello kafka", cfg.message)
	env := func(key string) string {
		return map[string]string{"KAFKA_BROKERS": "localhost:9093", "KAFKA_TOPIC": "env-topic"}[key]
	}
	cfg, err = parseConfig([]string{"-env-file", path}, io.Discard, env)
	require.NoError(t, err)
	require.Equal(t, "env-topic", cfg.topic)
	cfg, err = parseConfig([]string{"-env-file", path, "-brokers", " localhost:9094, [::1]:9095 ", "-topic", "arg-topic", "-message", "body", "-key", "key", "-count", "2"}, io.Discard, env)
	require.NoError(t, err)
	require.Equal(t, []string{"localhost:9094", "[::1]:9095"}, cfg.brokers)
	require.Equal(t, "arg-topic", cfg.topic)
	require.Equal(t, "body", cfg.message)
	require.Equal(t, 2, cfg.count)
}

func TestInvalidConfig(t *testing.T) {
	base := []string{"-brokers", "localhost:9092", "-topic", "test"}
	for _, args := range [][]string{
		{"-brokers", ""}, {"-brokers", "localhost:0"}, {"-brokers", "localhost:65536"}, {"-brokers", "localhost"}, {"-brokers", ":9092"},
		{"-brokers", "localhost:9092,"}, {"-topic", ""}, {"-topic", ".."}, {"-topic", "a b"}, {"-topic", strings.Repeat("a", 250)},
		{"-count", "0"}, {"-count", "10001"}, {"-count", "bad"}, {"-timeout", "0s"}, {"-timeout", "bad"},
		{"-env-file", "missing.env"}, {"-message", strings.Repeat("x", 1<<20)}, {"extra"}, {"-unknown"},
	} {
		_, err := parseConfig(append(append([]string{}, base...), args...), io.Discard, func(string) string { return "" })
		require.Error(t, err)
	}
	require.True(t, validTopic("a-Z_09.test"))
	require.False(t, validTopic("."))
	path := filepath.Join(t.TempDir(), "invalid.env")
	require.NoError(t, os.WriteFile(path, []byte("SECRET='private"), 0600))
	_, err := parseConfig(append(base, "-env-file", path), io.Discard, os.Getenv)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private")
}

func TestExecuteHelpAndCanceled(t *testing.T) {
	var out bytes.Buffer
	err := execute(context.Background(), []string{"-help"}, &out, func(string) string { return "" })
	require.Error(t, err)
	require.Contains(t, out.String(), "-brokers")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = execute(ctx, []string{"-brokers", "127.0.0.1:1", "-topic", "test"}, io.Discard, func(string) string { return "" })
	require.True(t, errors.Is(err, context.Canceled))
}

func TestProducerCLIHelper(t *testing.T) {
	if os.Getenv("D28_PRODUCER_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"kafka_producer"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestCLIInvalidConfigExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProducerCLIHelper$", "--", "-brokers", "bad", "-topic", "test")
	cmd.Env = append(os.Environ(), "D28_PRODUCER_HELPER=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err())
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 1, exit.ExitCode())
	require.Contains(t, string(out), "brokers")
}
func TestProduceTimeout(t *testing.T) {
	// 本地监听但不接受 Kafka 请求，确定性验证传输失败有界返回。
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cfg := producerConfig{brokers: []string{listener.Addr().String()}, topic: "test", count: 1, timeout: 100 * time.Millisecond}
	require.Error(t, produce(ctx, cfg, io.Discard))
	require.NoError(t, ctx.Err(), "必须由生产者期限结束")
}

func TestContinuousConfig(t *testing.T) {
	base := []string{"-brokers", "localhost:9092", "-topic", "test", "-continuous"}
	cfg, err := parseConfig(base, io.Discard, func(string) string { return "" })
	require.NoError(t, err)
	require.True(t, cfg.continuous)
	require.Equal(t, time.Second, cfg.interval)
	for _, extra := range [][]string{{"-count", "1"}, {"-interval", "0s"}, {"-interval", "-1s"}} {
		_, err := parseConfig(append(append([]string{}, base...), extra...), io.Discard, func(string) string { return "" })
		require.Error(t, err)
	}
}
func TestSendLoopContinuousLifetimeAndCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cfg := producerConfig{continuous: true, count: 1, timeout: 10 * time.Millisecond, interval: 15 * time.Millisecond}
	var calls int
	start := time.Now()
	err := sendLoop(ctx, cfg, func(attempt context.Context) error {
		_, ok := attempt.Deadline()
		require.True(t, ok)
		require.NoError(t, attempt.Err())
		calls++
		if calls == 3 {
			cancel()
		}
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 3, calls, "持续模式不受默认 count=1 限制")
	require.Greater(t, time.Since(start), cfg.timeout, "持续总时长可超过单条期限")
}
func TestSendLoopFiniteAndFailure(t *testing.T) {
	calls := 0
	require.NoError(t, sendLoop(context.Background(), producerConfig{count: 2}, func(context.Context) error { calls++; return nil }))
	require.Equal(t, 2, calls)
	failure := errors.New("send failed")
	err := sendLoop(context.Background(), producerConfig{continuous: true, timeout: time.Second, interval: time.Second}, func(context.Context) error { return failure })
	require.ErrorIs(t, err, failure)
	err = sendLoop(context.Background(), producerConfig{continuous: true, timeout: time.Millisecond, interval: time.Second}, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
func TestContinuousSignalExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("真实 SIGTERM 在 Linux 验证")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(5*time.Second)))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProducerCLIHelper$", "--", "-brokers", listener.Addr().String(), "-topic", "test", "-continuous", "-timeout", "10s")
	cmd.Env = append(os.Environ(), "D28_PRODUCER_HELPER=1")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	require.NoError(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	// 等待真实连接，确认子进程已注册信号处理后再发送 SIGTERM。
	conn, err := listener.Accept()
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	require.NoError(t, cmd.Wait(), output.String())
	require.NoError(t, ctx.Err())
}
