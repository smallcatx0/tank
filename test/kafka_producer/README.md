# Kafka 生产者测试程序

用于向明确指定的测试主题发送消息，与现有 worker 或自动化消费测试联调。使用项目已有 franz-go 依赖，当前支持明文 Kafka；不提供 SASL/TLS 参数。

## 手工生产与消费

先准备 Kafka 测试集群及主题。生产者不会自动创建主题，也不会修改消费者组。

从仓库根目录运行：

```powershell
go run ./test/kafka_producer -env-file .local/.env.dev -message "hello kafka" -key "example-key" -count 3
```

env 文件是可选的，支持 dotenv 格式，仅使用 KAFKA_BROKERS 和 KAFKA_TOPIC；不读取其中的消费者组作为生产参数。
地址和主题优先级：显式 -brokers/-topic > 进程环境变量 > -env-file。未配置则报错，不隐式连接任何本机服务。
具体连接信息留在本地忽略文件中，不提交。

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| -env-file | 空 | 显式指定配置文件，不自动导入进程环境 |
| -brokers | 从环境/文件读取 | 逗号分隔 host:port |
| -topic | 从环境/文件读取 | 已存在的测试主题 |
| -key | 空 | 所有发送消息使用相同 key |
| -message | hello kafka | 所有发送消息使用相同正文 |
| -count | 1 | 1..10000 |
| -timeout | 15s | 有限模式的总期限；持续模式的单条发送期限 |
| -continuous | false | 持续发送直到取消或失败，不能同时指定 -count |
| -interval | 1s | 持续模式成功发送后的等待间隔，必须为正 |

同步逐条发送，Broker 确认后每条输出一行 JSON，包含 topic、partition、offset，不输出 key 或正文。
保留 SDK 默认幂等生产，要求所有 ISR 确认；这不保证业务端恰好一次处理。
失败退出非零；此前已确认的消息不会回滚，输出失败也不代表消息未投递。
Ctrl+C/SIGTERM 取消发送并关闭客户端，正常退出；未确认消息仍可能已到达 Broker。

持续发送（首条立即发送，随后每次确认成功后等待 1 秒，不受 15 秒总时长限制）：

```powershell
go run ./test/kafka_producer -env-file .local/.env.dev -continuous -interval 1s -message "hello kafka" -key "manual-test"
```

持续模式发送失败也会退出，不自动无限重试。有限模式仍使用 -count，不应用发送间隔。

手工联调时，复制并修改 kafka_worker 配置，确保 brokers/topics 与生产者一致，并使用独立测试 group_id：

```powershell
if (!(Test-Path conf/kafka_worker.yaml)) { Copy-Item conf/kafka_worker.yaml.example conf/kafka_worker.yaml }
# 编辑 conf/kafka_worker.yaml 后，在另一个终端启动：
go run ./cmd/kafka_worker -config conf/kafka_worker.yaml
```

worker 示例只记录消息位置并提交。业务接入和关闭行为见 [Kafka 说明](../../pkg/kafka/README.md#worker-入口装配)。

## 自动化集成测试

明确设置 KAFKA_TEST_BROKERS 为可访问的专用测试集群地址后运行：

```powershell
go test ./test/kafka_producer -run '^TestIntegrationProducerConsumer$' -count=1 -timeout=90s -v
```

未设置该变量时明确跳过。测试独立生成 producer-d28-* 主题和消费者组，不使用 env 文件中的业务主题/组。
需要创建/删除主题、生产、消费、提交位点及删除消费者组权限。
复用现有 Broker；测试仅创建并清理自己的主题与组，不创建、重启或删除 Docker 容器。
创建主题使用一个分区、一份副本，适合单节点测试环境。

验证过程：
1. 真实生产者子进程发送 3 条消息，逐条核验回执、正文、key、partition 和 offset。
2. 通过 bootstrap.MustKafkaWorker 消费并排空，确保提交完成。
3. 同组重启后发送第 4 条消息，确认从新位点继续消费。
4. 清理测试资源并检查 Kafka 错误码。

```powershell
go test ./test/kafka_producer -count=1
go vet ./test/kafka_producer
go build ./test/kafka_producer
```

Go 竞态检查需要可用的 C 编译器；具备条件时执行 go test -race ./test/kafka_producer。
