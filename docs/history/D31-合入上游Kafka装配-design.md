<!-- D31 | 状态：已完成 -->
# 合入上游 Kafka 装配并新增 kafka_worker 入口

## 问题

上游模板仓库 `go-serv-tpl` 在提交 `8c77ba0` 中把 Kafka 消费能力装配进其 worker 入口，同时补充生产联调工具、4 篇设计文档与配置模板。本仓库在 D26 中已移植 `pkg/kafka` 封装，但装配层（`internal/bootstrap`、`internal/global`、`internal/task`）与入口尚未接入。

差异点是：上游的 `cmd/sth_worker` 是**纯 Kafka 示例入口**（不连 MySQL/Redis），本仓库的 `cmd/sth_worker` 是**业务任务入口**（装配 MySQL/Redis + RabbitMQ 任务）。直接把上游入口内容合进来，会让两个语义互相牵制。

实测证据：把 Kafka 装配合入本仓库 `cmd/sth_worker` 后，上游用例 `TestEntryStartupPanic/invalid-kafka-options` 失败——它在无存储配置下期望 `panic: kafka: Brokers`，而本仓库入口在 Kafka 装配之前先初始化 MySQL，实际 panic 为「mysql 初始化连接失败」，配置错误被存储失败掩盖。该用例的隐含前提是「入口不连存储」，与本仓库业务入口相反。

## 规则与约束

遵循 [工程规范](../../项目规范.md) 与 [工作流](../../AGENTS.md)。同步按既有冲突规则执行：拒绝模板对业务模块的删除、守住本地化模块路径、`go.mod` 不整文件覆盖。

## 方案与决策记录

- **入口拆分，不改造业务入口**：新增 `cmd/kafka_worker` 承接上游 Kafka 消费入口（日志 + 可选心跳 + 消费者），`cmd/sth_worker` 回退为业务任务入口（存储 + RabbitMQ 任务），两者互不引用。上游的入口代码与 `main_test.go` 整体迁入新入口，测试前提重新成立。
- **配置拆分**：`conf/worker.yaml.example` 恢复为任务配置（不含 `kafka` 段）；新增 `conf/kafka_worker.yaml.example` 承接上游示例（`enable: true`，`shutdown_timeout: 45`，`drain_timeout: 40s`）。示例值与 `drain_timeout` 校验口径一致：必须大于 35s（消息 30s + 提交 5s）且小于 `shutdown_timeout`。
- **编号重编**：上游归档文档使用 D26–D29，与本仓库已有的 D26（合入迭代）冲突。整体上移一位重编为 D27–D30，并同步修正各文件首行编号注释；本仓库最大编号推进到 D31。
- **校验前移**：`pkg/kafka` 新增导出 `Validate(Options) error`，`LoadKafkaConfig` 在解析完成后调用，使 broker/topic/group 缺失在构造消费者之前暴露。上游默认 `client_id` 为 `sth-worker`，入口拆分后语义不符，改为 `kafka-worker`，同步修正单测断言。
- **拒绝模板收缩**：上游把 readme 与配置改成「worker 不连接 MySQL/Redis」，与本仓库业务入口不符。保留业务描述，并把该表述限定到 `cmd/kafka_worker`；`pkg/` 目录清单补回 Excel 导出与任务调度。
- **本地排除方式按实际修正**：上游 AGENTS.md 称 `.local/` 由 `.git/info/exclude` 排除，本仓库实际以 `.local/.gitignore`（内容 `**`）自排除，按实际改写，避免后续误用 `git add -f`。

## 状态勾点（关键里程碑）

- [x] 核对上游 `8c77ba0` 的 27 个文件改动范围
- [x] 解决 4 处合并冲突（入口、配置、`curr.md`、`plan.md`）
- [x] 拆分入口并迁移配置与测试
- [x] 文档、编号、交叉引用同步
- [x] 全量回归

## 验收结论

2026-11-06：实现与自动验证通过，按收尾流程归档。

- `go build ./...`、`go vet ./...` 通过。
- `cmd/api`、`cmd/kafka_worker`、`cmd/sth_worker`、`internal/bootstrap`、`internal/global`、`pkg/kafka`、`test/kafka_producer` 测试全部通过。
- 全量 `go test ./...` 仅两条存量失败：`models/valid Test_JWTParse`、`models/dao/slscal Test_UpSLSData`，与改动前基线一致。
- `go test -race` 需 cgo，本机无 C 编译器，未执行；与既有环境限制相同。
- `git grep gitee.com` 仅命中 `docs/history` 中 D25 正文原貌与 D26 的变更说明，属于刻意保留的历史记载。

## 讨论与变更记录

- 初版按「合入单一入口」实现（`cmd/sth_worker` 内融合 Kafka 装配），用户指出应由独立入口承接冲突的 `sth_worker`，遂改为三入口方案。该决定同时消除了上游快速失败用例与本地存储前置的冲突，无需再为测试调整校验顺序。
