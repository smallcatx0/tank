package routes

import (
	versionv1 "gitee.com/chengduzhihuige/go-serv-template/sdk/version/v1"
	"serv_tpl/controller/rpc"

	"google.golang.org/grpc"
)

// RegisterGRPC 集中注册业务 RPC，供启动装配在监听前调用。
func RegisterGRPC(server grpc.ServiceRegistrar) {
	// 步骤1：在此维护业务服务及其依赖，统一向服务器注册。
	versionv1.RegisterVersionServiceServer(server, &rpc.VersionService{})
}
