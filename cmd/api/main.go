// Command api 顺序启动 HTTP 及可选 gRPC 服务，启动失败由 panic 终止进程。
package main

import (
	"os"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
)

func main() {
	// 步骤1：解析参数并加载配置
	global.InitFlag()
	if !global.Flag() {
		return
	}
	global.MustInitAppConf(global.Param.C)
	cfg := global.MustLoadLifecycleConfig(global.AppConf)

	// 步骤2：初始化日志和存储
	logSync := bootstrap.MustInitLog()
	closeDB := bootstrap.MustInitDBs(cfg.Startup)

	// 步骤3：启动心跳。
	bootstrap.Heartbeat(cfg.Heartbeat, cfg.HeartbeatInterval)

	// 步骤4：启动 HTTP 与可选 gRPC 服务。
	server := bootstrap.MustStartHTTP(cfg)
	rpc := bootstrap.MustGrpcServ(cfg)

	// 步骤5：全部启动成功后才接管退出，按显式顺序关闭，日志最后。
	os.Exit(bootstrap.WaitingExit(cfg.Shutdown, server.Done(), rpc.Done(),
		bootstrap.Cleanup{Name: "http", Stop: server.Shutdown},
		bootstrap.Cleanup{Name: "grpc", Stop: rpc.Shutdown},
		bootstrap.Cleanup{Name: "storage", Stop: closeDB},
		bootstrap.Cleanup{Name: "log", Stop: logSync},
	))
}
