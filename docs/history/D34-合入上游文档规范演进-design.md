<!-- D34 | 状态：已完成 -->
# 合入上游文档规范演进（Bash 统一与指南精简）

## 问题

上游仓库提交 `0be6dca` 统一文档命令为 Bash 语法、精简 `业务接入指南`，并新增归档文档 D30、D31——与本仓库已有的 D30（Kafka 生产者持续发送）、D31（合入上游 Kafka 装配）撞号。

同时存在一处实质张力：上游规则要求「不使用 PowerShell 命令示例」，而本仓库在 Windows 上的生成脚本 `scripts/generate-sdk.ps1` 是既有能力，且在 Restricted 执行策略下需要显式 `-ExecutionPolicy Bypass`，直接删除其文档入口会让本机流程失去依据。

## 规则与约束

遵循 [工程规范](../../项目规范.md)、[工作流](../../AGENTS.md) 与同步冲突规则：拒绝模板对业务内容的删除、守住本地化模块路径、不回退入口命名。

## 方案与决策记录

- **采纳 Bash 统一**：各 README、接入指南的代码块改为 `bash`，复制文件、环境变量、续行统一 Bash 写法；Windows 侧以 Git Bash 或 WSL 执行。HTTP 调用统一 `curl`。
- **保留 PowerShell 备选**：`scripts/generate-sdk.ps1` 在 AGENTS.md 与 SDK 说明中以「没有 Bash 环境时的备选」出现，并注明 Restricted 策略需显式 Bypass；不写成推荐示例，避免规则与本机能力断裂。
- **入口与路径不回退**：合并中保留 `cmd/kafka_worker`、`conf/kafka_worker.yaml`、`serv_tpl/sdk`，只替换代码块语言与命令写法，不采纳上游的 `sth_worker` 与 gitee 模块路径。
- **编号重编**：上游 D30、D31 重编为 D32、D33，同步修正首行编号与 `plan.md` 链接；本仓库最大编号推进到 D34。
- **plan.md 只记本仓库决策**：不搬运上游历史条目（其 D26–D29），仅补 D32、D33、D34。

## 状态勾点（关键里程碑）

- [x] 解决 5 处文档冲突（`pkg/kafka/README.md`、`plan.md`、`sdk/README.md`、`test/kafka_producer/README.md`、`业务接入指南.md`）
- [x] 编号重编与链接同步
- [x] Bash/PowerShell 口径一致性检查

## 验收结论

2026-11-06：文档冲突全部解决，检查通过。

- 冲突标记计数为 0。
- `gitee.com` 仅命中 `docs/history` 中 D25 正文原貌、D26 与 D31 的说明文字，属刻意保留的历史记载。
- `PowerShell` 提及仅出现在 AGENTS.md 规范条文、SDK 说明的 ps1 备选、生成脚本注释与 `curr.md` 现状描述中，无 PowerShell 命令示例残留。
- 本次仅文档与归档改动，未修改 Go 代码；`go build ./...` 与既有测试结果不受影响。
