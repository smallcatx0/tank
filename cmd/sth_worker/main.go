// Command sth_worker 消费者进程：装配存储后启动任务消费，收到信号按顺序优雅退出。
package main

import (
	"context"
	"log"
	"os"

	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/internal/task"
)

func main() {
	// 步骤1：解析参数并加载配置
	global.InitFlag()
	if !global.Flag() {
		return
	}
	global.MustInitAppConf(global.Param.C)
	cfg := global.MustLoadWorkerLifecycleConfig(global.AppConf)

	// 步骤2：初始化日志和存储
	logSync := bootstrap.MustInitLog()
	closeDB := bootstrap.MustInitDBs(cfg.Startup)

	// 步骤3：启动心跳
	bootstrap.Heartbeat(cfg.Heartbeat, cfg.HeartbeatInterval)

	// 步骤4：启动任务消费（数据库任务默认关闭，仅启用 RabbitMQ 任务）
	// dbClose := task.StartSthTask()
	rmqClose := task.StartRmqTask()
	log.Printf("task worker run")

	// 步骤5：等待退出信号，按显式顺序关闭：任务 -> 存储 -> 日志
	os.Exit(bootstrap.WaitingExit(cfg.Shutdown, nil, nil,
		bootstrap.Cleanup{Name: "rmq-task", Stop: func(context.Context) error { rmqClose(); return nil }},
		// bootstrap.Cleanup{Name: "db-task", Stop: func(context.Context) error { dbClose(); return nil }},
		bootstrap.Cleanup{Name: "storage", Stop: closeDB},
		bootstrap.Cleanup{Name: "log", Stop: logSync},
	))
}
