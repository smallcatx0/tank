// Command api 显式装配业务资源，可选观测由 bootstrap 管理。
package main

import (
	"os"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
)

func main() {
	// 步骤1：加载并校验必需的业务配置。
	global.InitFlag()
	if !global.Flag() {
		return
	}
	global.MustInitAppConf(global.Param.C)
	cfg := global.MustLoadLifecycleConfig(global.AppConf)

	// 步骤2：初始化日志和存储，按需启用观测；观测失败降级。
	logSync := bootstrap.MustInitLog()
	closeDB := bootstrap.MustInitDBs(cfg.Startup)
	// 步骤3：启动观测（非必须，配置失败不会阻塞主流程）
	observation := bootstrap.StartObservability(global.AppConf, cfg, cfg.Drain, cfg.GRPC.Drain)

	// 步骤4：显式启动业务入口并登记关闭顺序。
	server := bootstrap.MustStartHTTP(cfg, observation.Runtime)
	rpc := bootstrap.MustGrpcServ(cfg, observation.Runtime)
	os.Exit(bootstrap.WaitingExit(cfg.Shutdown, server.Done(), rpc.Done(), nil,
		bootstrap.Cleanup{Name: "http", Stop: server.Shutdown},
		bootstrap.Cleanup{Name: "grpc", Stop: rpc.Shutdown},
		bootstrap.Cleanup{Name: "storage", Stop: closeDB},
		bootstrap.Cleanup{Name: "observability", Stop: observation.Shutdown},
		bootstrap.Cleanup{Name: "log", Stop: logSync},
	))
}
