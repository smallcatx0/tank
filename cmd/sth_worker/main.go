// Command sth_worker 保留示例任务，退出协议仅适配基础生命周期。
package main

import (
	"context"
	"errors"
	"os"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/internal/task"
)

func main() {
	// 步骤1：解析启动参数，帮助信息不进入服务生命周期。
	global.InitFlag()
	if !global.Flag() {
		return
	}
	// 步骤2：先监听退出信号；启动失败或 panic 同样回滚已登记的资源。
	// Run 完成清理后返回退出码，不能依赖 main 的 defer 关闭资源。
	os.Exit(bootstrap.Run(start))
}

// start 显式装配本进程资源，生产入口与测试共用同一条启动路径。
func start(ctx context.Context, r *bootstrap.Runtime) error {
	// 步骤3：加载并校验 worker 配置，不读取 HTTP 配置。
	global.MustInitAppConf(global.Param.C)
	cfg, err := global.LoadWorkerLifecycleConfig(global.AppConf)
	if err != nil {
		return err
	}
	r.SetShutdownTimeout(cfg.Shutdown)

	// 步骤4：初始化全局日志，最先登记以确保最后关闭。
	logSync := bootstrap.MustInitLog()
	r.Add("log", logSync)

	// 步骤5：在启动期限内连接 MySQL、Redis，成功后立即登记释放。
	startup, cancel := context.WithTimeout(ctx, cfg.Startup)
	defer cancel()
	closeDB, err := bootstrap.InitDBs(startup)
	cancel()
	if err != nil {
		return err
	}
	r.Add("storage", closeDB)
	if err = ctx.Err(); err != nil {
		return err
	}

	// 步骤6：按配置启用心跳，退出时停止采样并等待结束。
	stopHeartbeat, err := bootstrap.StartHeartbeat(bootstrap.HeartbeatOptions{
		Enabled: cfg.Heartbeat, Interval: cfg.HeartbeatInterval, OnFailure: r.Fail,
	})
	if err != nil {
		return err
	}
	r.Add("heartbeat", stopHeartbeat)

	// 步骤7：仅启动后台任务，不启动 HTTP 服务。
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := task.StartSthTask()
	if stop == nil {
		return errors.New("示例任务启动失败")
	}
	r.Add("task", func(context.Context) error { stop(); return nil })
	// 步骤8：交还 Run 等待退出，按任务→心跳→存储→日志清理。
	return nil

}
