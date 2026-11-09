# Kafka 消费组件

`pkg/kafka` 提供单条回调、业务重试、逐条同步提交与优雅关闭。设计和验收边界见 [D22](../../docs/history/D22-Kafka消费能力封装-design.md)。HTTP 入口不启用 Kafka；worker 的装配示例见下文。

## 最小运行示例

先在目标测试集群中创建 example-events 主题（组件不会自动创建主题）。下面的 broker 地址仅为示例，需替换为应用可访问的地址。

将下面程序保存到仓库内的独立目录，再使用 `go run` 运行。示例持续消费，收到 Ctrl+C 后排空在途消息。

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "runtime/debug"
    "syscall"
    "time"

    "go.uber.org/zap"
    "serv_tpl/pkg/kafka"
    "serv_tpl/pkg/logger"
)

func main() {
    o := kafka.DefaultOptions()
    o.Brokers = []string{"127.0.0.1:9092"} // 按目标环境修改
    o.GroupID = "example-worker"
    o.Topics = []string{"example-events"}
    o.StartOffset = "earliest"
    consumer, err := kafka.NewConsumer(o)
    if err != nil { logger.L().Error("创建消费者失败", zap.Error(err)); return }

    signals := make(chan os.Signal, 1)
    signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
    defer signal.Stop(signals)
    done := make(chan error, 1)
    go func() {
        defer func() {
            if p := recover(); p != nil {
                logger.L().Error("消费任务 panic", zap.ByteString("stack", debug.Stack()))
                done <- context.Canceled
            }
        }()
        done <- consumer.Run(context.Background(), func(ctx context.Context, m kafka.Message) error {
            // 业务处理必须响应 ctx；成功返回前完成副作用，并自行保证幂等。
            logger.L().Info("收到消息", zap.String("topic", m.Topic), zap.Int64("offset", m.Offset))
            return nil
        })
    }()
    select {
    case err := <-done:
        if err != nil { logger.L().Error("消费者退出", zap.Error(err)) }
    case <-signals:
        ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
        defer cancel()
        if err := consumer.Shutdown(ctx); err != nil {
            logger.L().Error("消费者排空失败", zap.Error(err))
            return
        }
        if err := <-done; err != nil { logger.L().Error("消费者退出", zap.Error(err)) }
    }
}
```

生产入口应使用项目现有装配机制初始化日志，并负责监听 Run 失败、总退出期限与二次信号强退。Shutdown 必须使用独立且带 deadline 的 context。回调不响应取消时无法强制终止回调；Shutdown 超时不代表 Run 或资源已回收。

## 指标接入

消费者实现 `prometheus.Collector`，构造时不注册任何全局指标：

```go
registry := prometheus.NewRegistry()
if err := registry.Register(consumer); err != nil {
    return err
}
// 使用 promhttp.HandlerFor(registry, promhttp.HandlerOpts{}) 挂载到应用自己的指标入口。
```

同一注册器内多个实例应使用不同 ClientID。需要移除指标时由注册者调用 `registry.Unregister(consumer)`。业务回调可从 Run 传入的 context 获取业务值；组件不会解析消息正文来推断 trace_id 或业务 ID。

## 验证

```bash
go test ./pkg/kafka
go test -race ./pkg/kafka
go test ./pkg/kafka -run '^$' -bench . -benchmem

export KAFKA_TEST_BROKERS='127.0.0.1:9092' # 示例，替换为专用测试集群地址
go test ./pkg/kafka -run TestIntegration -count=1 -timeout=150s
unset KAFKA_TEST_BROKERS
```

未设置环境变量时真实 Broker 测试会明确跳过。集成测试创建独立的 `gtank-d22-*` 主题与消费者组，结束后清理自己的测试数据；应只对测试集群运行。`BenchmarkConsumerRound` 使用内存提交替身，测量组件调度开销，不代表真实 Kafka 吞吐。

### 真实认证与网络故障专项

```bash
export KAFKA_DOCKER_TESTS='1'
go test ./pkg/kafka -run '^TestDockerAuthenticationAndNetworkFaults$' -count=1 -timeout=300s -v
unset KAFKA_DOCKER_TESTS
```

需要可访问的本机 Docker daemon。测试自行创建独立 `apache/kafka:4.1.1` Broker，动态选择本机端口，生成短期 CA、服务端证书和随机测试凭据；通过真实协议代理制造连接中断、请求/响应丢失和心跳失联，并强制终止、重启自有 Broker。结束后清理自有容器及测试凭据，不读取或改动常驻 Kafka 配置。该套件也在自有 Broker 上运行常规集成回归，不需要设置 `KAFKA_TEST_BROKERS`。逐项结果见 [D22 验收结论](../../docs/history/D22-Kafka消费能力封装-design.md#验收结论)。

## worker 入口装配

先准备独立示例主题，从仓库根目录复制配置（已有配置不覆盖），再按目标环境修改 brokers、topics 与 group_id：

```bash
test -f conf/worker.yaml || cp conf/worker.yaml.example conf/worker.yaml
# 编辑 conf/worker.yaml 后启动
go run ./cmd/sth_worker -config conf/worker.yaml
```

kafka.enable 默认 false；worker.yaml.example 显式开启 Kafka，其中地址仅为示例。回调位于 internal/task/kafka.go，
只记录消息位置，成功后提交位点，请仅使用示例主题。替换为真实处理器时应在返回 nil 前完成业务并保证幂等。
消费失败停止进程并返回非零退出码；Ctrl+C/SIGTERM 先排空消费者，最后关闭日志。
排空预算默认 40s，总退出预算示例为 45s；二次信号或总超时强退。
启动日志不代表 Broker 已就绪，主题需预先创建。worker 可通过 `observability.metrics.enable` 启用独立监控 HTTP，并在 Run 前注册本 Collector；追踪由内部包装层处理，组件保持独立。配置与验证见 [观测接入](../../internal/telemetry/README.md)。

可运行的生产者及生产消费集成测试见 [生产者测试程序](../../test/kafka_producer/README.md)。
