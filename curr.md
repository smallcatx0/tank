# 当前现状

项目当前为init 阶段。不用考虑变更风险与兼容问题。

- Kafka 消费组件提供逐条提交、有限重试、优雅关闭与显式注册指标，已接入可选 worker 示例入口（仅记录消息位置并提交）；用法见 [组件说明](pkg/kafka/README.md)。
- RPC 契约生成代码位于独立 `sdk/` Go module，服务端与客户端共用；生成支持 PowerShell 与 Bash，生成、发布与调用见 [SDK 说明](sdk/README.md)。

- 提供 [Kafka 生产者及生产消费集成测试](test/kafka_producer/README.md)，支持显式参数、env 文件配置及带间隔的持续生产。

- API/worker 支持独立开关的 Prometheus 与 OpenTelemetry，默认关闭；配置、业务调用与部署验证见 [观测接入](internal/telemetry/README.md)。

# 已知问题

- 独立 SDK 尚未发布远程版本。
