package bootstrap

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func heartbeatRunning() bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Contains(string(buf[:n]), "bootstrap.startHeartbeat.func1")
}

func TestStartHeartbeatSkipped(t *testing.T) {
	for _, tc := range []struct {
		name     string
		enabled  any
		interval any
	}{
		{"defaults", nil, nil},
		{"disabled", false, "invalid"},
		{"invalid_switch", "invalid", 10},
		{"zero", true, 0},
		{"negative", true, -1},
		{"invalid_interval", true, "invalid"},
		{"overflow", true, int64(9223372036854775807)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := viper.New()
			if tc.enabled != nil {
				c.Set("metrics.enable", tc.enabled)
			}
			if tc.interval != nil {
				c.Set("metrics.dt", tc.interval)
			}
			require.NotPanics(t, func() { startHeartbeat(c) })
			require.False(t, heartbeatRunning())
			if tc.interval == nil {
				require.Equal(t, int64(10), c.GetInt64("metrics.dt"))
			}
		})
	}
}

// 进程级心跳没有关闭句柄，成功启动用子进程验证，避免泄漏到其他测试。
func TestStartHeartbeatEnabled(t *testing.T) {
	if os.Getenv("TEST_BOOTSTRAP_HEARTBEAT") == "1" {
		c := viper.New()
		c.Set("metrics.enable", "true")
		startHeartbeat(c)
		require.Equal(t, int64(10), c.GetInt64("metrics.dt"))
		require.Eventually(t, heartbeatRunning, time.Second, time.Millisecond)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartHeartbeatEnabled$", "-test.timeout=10s")
	cmd.Env = append(os.Environ(), "TEST_BOOTSTRAP_HEARTBEAT=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}
