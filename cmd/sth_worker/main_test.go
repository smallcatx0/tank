package main

import (
	"context"
	"flag"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestEntryHelper(t *testing.T) {
	if os.Getenv("D02_ENTRY_HELPER") != "1" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("entry", flag.ExitOnError)
	os.Args = []string{"entry", "-config", os.Getenv("D02_ENTRY_CONFIG")}
	main()
}
func entryCommand(t *testing.T, config string) (*exec.Cmd, context.Context) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "app.yaml")
	require.NoError(t, os.WriteFile(file, []byte(config), 0600))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEntryHelper$")
	cmd.Env = append(os.Environ(), "D02_ENTRY_HELPER=1", "D02_ENTRY_CONFIG="+file)
	return cmd, ctx
}
func TestEntryStartupPanic(t *testing.T) {
	for _, tc := range []struct{ name, config, want string }{
		{"invalid-http-ignored", "shutdown_timeout: 1\nhttp:\n  port: invalid\n  drain_timeout: invalid\nmysql:\n  dsn: secret-password\n", "mysql DSN 无效"},
		{"invalid-common", "shutdown_timeout: 0\n", "配置 shutdown_timeout"},
		{"invalid-config", "mysql: [secret-password\n", "配置加载失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, ctx := entryCommand(t, tc.config)
			out, err := cmd.CombinedOutput()
			require.NoError(t, ctx.Err(), "启动应迅速失败，不应等待退出信号")
			var exited *exec.ExitError
			require.ErrorAs(t, err, &exited)
			require.NotZero(t, exited.ExitCode())
			require.Contains(t, string(out), "panic: "+tc.want)
			require.NotContains(t, string(out), "secret-password")
			require.NotContains(t, string(out), "启动 panic", "不能统一恢复启动 panic")
		})
	}
}
