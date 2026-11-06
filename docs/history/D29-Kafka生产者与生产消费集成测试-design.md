<!-- D29 | 状态：已完成 -->
# Kafka生产者与生产消费集成测试

## 问题
用户要求在 test 下提供可执行 Kafka 生产者，并使用本地 Docker Kafka 完成生产消费集成测试。

## 规则与约束
遵循 [项目规范](../../项目规范.md)、[工作流](../../AGENTS.md) 及本地测试隔离约定。既有 worker 改动保留。

## 方案与决策记录
- 新增 test/kafka_producer，使用已有 franz-go 依赖，不扩展 pkg/kafka 的公共 API。
- 提供 -brokers、-topic、-key、-message、-count、-timeout、-env-file。地址和主题优先级为显式参数 > 进程环境变量 > 显式指定 env 文件，无隐式本机地址。
- env 文件只读取 KAFKA_BROKERS/KAFKA_TOPIC，不载入进程环境或记录其他服务凭据；KAFKA_GROUP_ID 是消费者配置，不用于生产。
- 默认发送 1 条 hello kafka，key 为空，全部发送总期限 15s。同步逐条发送、all ISR ack，保留 SDK 默认幂等生产；输出 topic/partition/offset，不输出正文或 key。
- 程序不自动建主题；部分发送后失败返回非零，已成功消息不会回滚。网络/投递有显式超时，取消信号终止后续发送并关闭客户端。
- 自动化测试显式要求 KAFKA_TEST_BROKERS，未设置时跳过；创建唯一单分区主题与组，执行真实生产者进程，通过 bootstrap 的消费者校验内容和顺序，排空后以同组重启并核验从新消息继续消费。
- 清理仅限本次临时主题与组，检查 Kafka 响应错误码；不创建或销毁常驻 Broker。
- 按用户授权恢复测试环境后执行验证；具体服务配置、数据挂载和修复记录只存本地，不提交机器专属信息。

## 状态勾点（关键里程碑）
- [x] 明确 CLI、隔离和验证范围
- [x] 实现生产者与参数测试
- [x] 本地真实生产消费集成验证
- [x] 验收归档

## 验收结论
2026-11-06：生产者及生产消费集成测试验收通过。
- go test ./...、生产者构建、相关 go vet、git diff --check 通过；未配置 Broker 时外部用例明确跳过。
- Linux Go 容器执行 go test -race ./test/kafka_producer 通过（不依赖 Broker 的路径）。
- TestIntegrationProducerConsumer 在真实 Kafka 上通过：真实生产者子进程发送前三条消息，核验正文、key、分区和 offsets 0..2；排空后同组重启，只消费新消息 offset 3。
- 独立主题和组删除成功；清理后 Broker 仍健康，无重启。测试不保留业务主题。
- 本机环境配置的修复经用户授权执行，细节在本地记录；未改动消费者公共接口或依赖版本。
