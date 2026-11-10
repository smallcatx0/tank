// Command kafka_worker 装配 Kafka 消费者并运行示例回调，不启动 HTTP/gRPC，不连接数据库与 Redis。
package main

import (
	"os"
	"time"

	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/internal/task"
)

func main() {
	// 步骤1：解析参数并加载 worker 配置，不校验 HTTP 与存储参数。
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

	// 步骤2：初始化日志，失败直接 panic；按需启用观测（含心跳），业务排空预算只计算启用项。
	logSync := bootstrap.MustInitLog()
	var drain time.Duration
	if kafkaCfg.Enabled {
		drain = kafkaCfg.Drain
	}
	observation := bootstrap.StartObservability(global.AppConf, cfg, drain)

	// 步骤3：启动可选 Kafka 消费者；enable=false 时不启动。
	consumer := bootstrap.MustKafkaWorker(kafkaCfg, task.LogKafkaMessage, observation.Runtime)

	// 步骤4：等待退出，先排空消费者、再关闭观测、最后关闭日志。
	os.Exit(bootstrap.WaitingWorkerExit(cfg.Shutdown, consumer.Done(), nil,
		bootstrap.Cleanup{Name: "kafka", Stop: consumer.Shutdown},
		bootstrap.Cleanup{Name: "observability", Stop: observation.Shutdown},
		bootstrap.Cleanup{Name: "log", Stop: logSync},
	))
}
