// Command sth_worker 显式装配后台业务，可选观测由 bootstrap 管理。
package main

import (
	"os"
	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/internal/task"
	"time"
)

func main() {
	// 步骤1：加载 worker 业务配置，不校验 HTTP 参数。
	global.InitFlag()
	if !global.Flag() {
		return
	}
	global.MustInitAppConf(global.Param.C)
	cfg := global.MustLoadWorkerLifecycleConfig(global.AppConf)
	kafkaCfg, err := global.LoadKafkaConfig(global.AppConf, cfg.Shutdown)
	if err != nil {
		panic(err)
	}

	// 步骤2：初始化日志和可选观测，业务排空预算只计算启用项。
	logSync := bootstrap.MustInitLog()
	var drain time.Duration
	if kafkaCfg.Enabled {
		drain = kafkaCfg.Drain
	}

	observation := bootstrap.StartObservability(global.AppConf, cfg, drain)

	// 步骤3：显式选择业务回调并登记关闭顺序。
	consumer := bootstrap.MustKafkaWorker(kafkaCfg, task.LogKafkaMessage, observation.Runtime)
	os.Exit(bootstrap.WaitingWorkerExit(cfg.Shutdown, consumer.Done(), nil,
		bootstrap.Cleanup{Name: "kafka", Stop: consumer.Shutdown},
		bootstrap.Cleanup{Name: "observability", Stop: observation.Shutdown},
		bootstrap.Cleanup{Name: "log", Stop: logSync},
	))
}
