<!-- D27 | 状态：已完成 -->
# Worker 装配 Kafka 消费

## 问题
[现状](../../curr.md)中的 Kafka 组件尚未接入 worker。用户要求在 cmd/sth_worker 装配消费。

## 规则与约束
遵循 [工程规范](../../项目规范.md) 与 [工作流](../../AGENTS.md)。

## 方案与决策记录
- worker 读取 kafka.enable（默认 false）；开启时 brokers、group_id、topics、start_offset 必填，由组件构造校验。client_id 默认 sth-worker。
- 从 kafka.DefaultOptions 构造，保持单条、单并发、同步提交、最多 3 次业务尝试及组件既有失败语义；暂不增加认证配置与高级调优配置。
- drain_timeout 默认 40s，必须大于组件单条业务与提交预算合计 35s，并小于进程 shutdown_timeout。独立 conf/worker.yaml.example 使用 45 秒进程预算，原 API 配置不受影响。
- bootstrap 装配并写入 global.KafkaCli；独立 goroutine 运行，发布结果，recover 并记录堆栈。NewConsumer 不保证 Broker 就绪。
- WaitingWorkerExit 复用现有退出看门狗与清理顺序，Run 意外返回（包含 nil）触发非零退出。信号关闭使用独立 context 调用 Shutdown，不提前取消 Run。
- 先关闭 Kafka 并等待包装协程，再关闭日志；超时由进程期限兜底。不重启失败消费者。
- internal/task 回调仅记录 topic/partition/offset，不记录正文或 key；成功即提交。只用于 example-events 与独立组，替换业务回调后需自行保证幂等。
- 启动与 Shutdown 竞争时，仅将包装层收到的直接 ErrClosed/ErrAlreadyRun 视为已关闭；不吞掉回调包装后的业务错误。Shutdown 等待 Run 包装层退出，防止日志关闭后仍记录运行结果。
- 本次不添加 HTTP 指标端口，不自动创建主题，不连接数据库或 Redis。

## 状态勾点（关键里程碑）
- [x] 核对组件与入口现状
- [x] 明确配置、装配与示例处理边界
- [x] 完成实现和测试
- [x] 人工审阅共享退出流程与示例消费边界

## 验收结论
2026-11-06：实现及自动验证通过，用户确认人工审阅完成，验收通过。
- go test ./...、go build ./cmd/...、go vet ./... 通过。
- Linux golang:1.26 容器执行 go test -race ./... 通过，覆盖既有信号、二次信号及看门狗回归。本机 Windows 缺少 CGO 编译器，故在容器执行。
- TestKafkaWorkerIntegration 在显式指定的本机测试 Broker 上通过：独立主题两次消费、排空、同组重启后从下一位点恢复；测试主题与组已清理。
- 新增配置解析、Kafka 装配、示例回调函数单元覆盖率均为 100%。核心 internal/pkg/数据连接与校验包汇总覆盖率 89.32%（排除 schema 纯 DTO）。
- 全仓原始覆盖率为 81.8%，含示例与入口；bootstrap 包 84.8%、resp 74.2%、dao 81.4%。在独立 HEAD 检出复跑确认，基线分别为 82.1%、74.2%、81.4%，属于已有覆盖缺口。
- staticcheck 未安装，未执行；未修改 go.mod/go.sum。
- 共享退出逻辑仅提取复用并增加 worker 结果分支；自动回归和人工审阅通过，已验收归档。
