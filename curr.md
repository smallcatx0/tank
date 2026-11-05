# 当前现状

项目当前为init 阶段。不用考虑变更风险与兼容问题。

- Kafka 消费组件提供逐条提交、有限重试、优雅关闭与显式注册指标，尚未接入业务入口；用法见 [组件说明](pkg/kafka/README.md)。
- 本机持久 Kafka 服务由 Docker Compose 管理，连接与管理方式见 [组件说明](pkg/kafka/README.md#本机持久-kafka)。
- RPC 契约生成代码位于独立 `sdk/` Go module，服务端与客户端共用；生成支持 PowerShell 与 Bash，生成、发布与调用见 [SDK 说明](sdk/README.md)。

# 已知问题

- 独立 SDK 尚未发布远程版本。

# 下一步
