package routes

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestRegisterGRPC(t *testing.T) {
	server := grpc.NewServer()
	t.Cleanup(server.Stop)
	RegisterGRPC(server)
	services := server.GetServiceInfo()
	version, ok := services["version.v1.VersionService"]
	require.True(t, ok, "业务注册入口应提供版本服务")
	require.Contains(t, version.Methods, grpc.MethodInfo{Name: "Version"})
	require.NotContains(t, services, "grpc.health.v1.Health", "生命周期健康服务由启动装配管理")
}
