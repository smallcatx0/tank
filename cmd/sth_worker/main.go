// Command sth_worker 顺序启动后台任务，不启动 HTTP。
package main

import (
	"context"
	"os"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/internal/task"
)

func main() {
	// 步骤1：解析参数并加载 worker 配置，不校验 HTTP 参数。
	global.InitFlag()
	if !global.Flag() {
		return
	}
	global.MustInitAppConf(global.Param.C)
	cfg := global.MustLoadWorkerLifecycleConfig(global.AppConf)

	// 步骤2：初始化日志和存储，失败直接 panic。
	logSync := bootstrap.MustInitLog()
	closeDB := bootstrap.MustInitDBs(cfg.Startup)

	// 步骤3：启动心跳和后台任务。
	bootstrap.Heartbeat(cfg.Heartbeat, cfg.HeartbeatInterval)
	stopTask := task.MustStartSthTask()

	// 步骤4：运行后的退出按任务、存储、日志顺序执行。
	os.Exit(bootstrap.WaitingExit(cfg.Shutdown, nil,
		bootstrap.Cleanup{Name: "task", Stop: func(context.Context) error { stopTask(); return nil }},
		bootstrap.Cleanup{Name: "storage", Stop: closeDB},
		bootstrap.Cleanup{Name: "log", Stop: logSync},
	))
}
