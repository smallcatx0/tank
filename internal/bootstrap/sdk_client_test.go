package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"serv_tpl/internal/global"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// TestGRPCIndependentSDKClient 验证独立模块只依赖 SDK 即可通过真实 TCP 调用服务。
func TestGRPCIndependentSDKClient(t *testing.T) {
	previous := global.AppConf
	t.Cleanup(func() { global.AppConf = previous })
	global.AppConf = viper.New()
	global.AppConf.Set("name", "sdk-client")
	global.AppConf.Set("ver", "1.0.0")
	server, _ := startTestGRPC(t, time.Second)

	// 步骤1：创建没有根模块依赖的外部项目，编译文档对应的客户端。
	dir := t.TempDir()
	sdkPath, err := filepath.Abs("../../sdk")
	require.NoError(t, err)
	mod := "module example.com/sdk-client-test\n\ngo 1.26.0\n\nrequire serv_tpl/sdk v0.0.0\nreplace serv_tpl/sdk => " + "\"" + filepath.ToSlash(sdkPath) + "\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600))
	// 使用已锁定的依赖校验和，离线回归不需要访问校验和服务。
	sums, err := os.ReadFile(filepath.Join(sdkPath, "go.sum"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), sums, 0600))
	source, err := os.ReadFile("../../test/grpc_version/main.go")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), source, 0600))
	run := func(t *testing.T, addr string, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GRPC_ADDR="+addr)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return out
	}
	run(t, "", "mod", "tidy")
	modules := string(run(t, "", "list", "-m", "all"))
	// 根模块为 serv_tpl、SDK 为 serv_tpl/sdk，按行首精确匹配确认未引入服务端根模块。
	for _, line := range strings.Split(strings.TrimSpace(modules), "\n") {
		require.False(t, strings.HasPrefix(line, "serv_tpl "), modules)
	}

	// 步骤2：检查环境变量和参数优先级，验证生成客户端与真实服务契约一致。
	for _, tc := range []struct {
		name string
		env  string
		args []string
	}{
		{"environment", server.listener.Addr().String(), []string{"run", ".", "-timeout", "3s"}},
		{"flag_overrides_environment", "127.0.0.1:1", []string{"run", ".", "-addr", server.listener.Addr().String(), "-timeout", "3s"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := run(t, tc.env, tc.args...)
			var result map[string]string
			require.NoError(t, json.Unmarshal(out, &result))
			require.Equal(t, map[string]string{"name": "sdk-client", "desc": "", "version": "1.0.0"}, result)
		})
	}
}
