# Kafka 消费组件

`pkg/kafka` 提供单条回调、业务重试、逐条同步提交与优雅关闭。设计和验收边界见 [D22](../../docs/history/D22-Kafka消费能力封装-design.md)。现有 HTTP/worker 入口没有自动启用 Kafka。

## 最小运行示例

先在本机 Kafka 中创建示例主题（组件不会自动创建主题）：

```powershell
docker exec gtank-kafka-4.1.1 /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:19092 --create --if-not-exists --topic example-events --partitions 1 --replication-factor 1
```

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
    o.Brokers = []string{"127.0.0.1:19092"}
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

## 本机持久 Kafka

- 容器：`gtank-kafka-4.1.1`，镜像：`apache/kafka:4.1.1`。
- Compose 文件：`D:\docker\kafka_4.1.1\compose.yaml`，项目名 `gtank-kafka`，服务名 `kafka`。
- 宿主机连接地址：`127.0.0.1:19092`，PLAINTEXT，仅绑定本机。
- 命名数据卷：`gtank-kafka-4.1.1-data`，挂载到 `/var/lib/kafka/data`。
- 数据卷声明为 external，必须预先存在；Compose 不负责创建或删除该卷。
- 自动重启策略：`unless-stopped`；手动停止后须手动启动。
- 单节点 KRaft，Broker/Controller 合并；事务与 offset 内部主题副本数均为 1。

```powershell
docker compose -f D:\docker\kafka_4.1.1\compose.yaml up -d --wait
docker compose -f D:\docker\kafka_4.1.1\compose.yaml stop
docker compose -f D:\docker\kafka_4.1.1\compose.yaml logs --tail 100 kafka
docker compose -f D:\docker\kafka_4.1.1\compose.yaml ps
```

该监听地址面向宿主机程序，不适用于其他容器直接连接；其他容器接入需另行配置可达的 advertised listener。验证不会删除该容器或数据卷。

## 验证

```powershell
go test ./pkg/kafka
go test -race ./pkg/kafka
go test ./pkg/kafka -run '^$' -bench . -benchmem

$env:KAFKA_TEST_BROKERS = '127.0.0.1:19092'
go test ./pkg/kafka -run TestIntegration -count=1 -timeout=150s
Remove-Item Env:KAFKA_TEST_BROKERS
```

未设置环境变量时真实 Broker 测试会明确跳过。集成测试创建独立的 `gtank-d22-*` 主题与消费者组，结束后清理自己的测试数据；应只对测试集群运行。`BenchmarkConsumerRound` 使用内存提交替身，测量组件调度开销，不代表真实 Kafka 吞吐。

### 真实认证与网络故障专项

```powershell
$env:KAFKA_DOCKER_TESTS = '1'
go test ./pkg/kafka -run '^TestDockerAuthenticationAndNetworkFaults$' -count=1 -timeout=300s -v
Remove-Item Env:KAFKA_DOCKER_TESTS
```

需要可访问的本机 Docker daemon。测试自行创建独立 `apache/kafka:4.1.1` Broker，动态选择本机端口，生成短期 CA、服务端证书和随机测试凭据；通过真实协议代理制造连接中断、请求/响应丢失和心跳失联，并强制终止、重启自有 Broker。结束后清理自有容器及测试凭据，不读取或改动常驻 Kafka 配置。该套件也在自有 Broker 上运行常规集成回归，不需要设置 `KAFKA_TEST_BROKERS`。逐项结果见 [D22 验收结论](../../docs/history/D22-Kafka消费能力封装-design.md#验收结论)。
