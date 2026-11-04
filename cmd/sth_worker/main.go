// Command sth_worker 顺序启动后台任务，不启动 HTTP。
package main

import (
	"os"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
)

func main() {
	// 步骤1：解析参数并加载 worker 配置，不校验 HTTP 参数。
	global.InitFlag()
	if !global.Flag() {
		return
	}
	global.MustInitAppConf(global.Param.C)
	cfg := global.MustLoadWorkerLifecycleConfig(global.AppConf)

	// 步骤2：初始化日志，失败直接 panic。
	logSync := bootstrap.MustInitLog()

	// 步骤3：启动可选心跳，后台任务按业务需要接入。
	bootstrap.Heartbeat(cfg.Heartbeat, cfg.HeartbeatInterval)

	// TODO 启动worker后台任务

	// 步骤4：等待退出，日志最后关闭。
	os.Exit(bootstrap.WaitingExit(cfg.Shutdown, nil, nil,
		bootstrap.Cleanup{Name: "log", Stop: logSync},
	))
}
