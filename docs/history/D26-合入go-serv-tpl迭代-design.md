<!-- D26 | 状态：已完成 -->
# 合入 go-serv-tpl 迭代（Kafka 封装与独立 SDK）

## 问题

以本仓库为基础创建的新项目 `go-serv-tpl` 在其上迭代了三项能力：Kafka 消费封装（D22/D24）、独立 Go 客户端 SDK（D25）、生成工具链与文档规范演进。该仓库为独立 `git init`，与本仓库无共同祖先，且其差异中约半数是「模板收缩」——删除了 user/admin 控制器、`models/valid`、`pkg/excel`、`pkg/sth_job`、`internal/task`、`httpmd/jwt_auth.go`、`sql/user.sql` 等业务代码。直接整树合并会把这些能力从本仓库删除，属于倒退。

目标：吸收三项有效能力，保留本仓库全部业务模块，并让后续 `go-serv-tpl` 的迭代可以增量同步。

## 规则与约束

- 工程决策遵循 [项目规范](../../项目规范.md)；流程与状态管理遵循 [AGENTS.md](../../AGENTS.md)。
- `pkg/` 必须零业务依赖、不得 import `internal/`：`pkg/kafka` 仅依赖 `pkg/logger` 与外部库，符合要求。
- 核心逻辑覆盖率门槛见项目规范第 7 节；本次新增代码随移植带上原有单元/并发/故障注入测试。
- 文档命名采用 `docs/编号-需求名称-design.md`，本次同时把该规则同步进 AGENTS.md 与项目规范。

## 方案与决策记录

**决策 1：用 `git merge -s ours --allow-unrelated-histories` 建立血缘，再按能力块移植。**

两仓库无共同祖先，直接合并会被拒绝；`--allow-unrelated-histories` 全量解冲突需逐文件决策 89 个文件。改用 `ours` 策略先记录一次空合并：工作区零改动，但把对方历史记为祖先。此后 `git merge tpl/main` 的三方基准就是本次状态，只会在真正改动的文件上产生冲突。代价是本仓库历史中会包含 `go-serv-tpl` 的提交记录。

**决策 2：只移植有效增量，拒绝模板收缩。**

采纳：`pkg/kafka/**`、`sdk/**`、`scripts/generate-sdk.{ps1,sh}`、`.gitattributes`、`test/grpc_version` 升级、`internal/bootstrap/sdk_client_test.go`、`api/version/v1/version.proto` 的 `go_package` 与 `api/README.md`、Dockerfile 的 SDK 依赖层、文档规范演进、D22/D24/D25 归档。

拒绝：`routes/router.go` 与 `internal/global/conf.go` 的精简版本（会删掉 user 路由与 `JwtSecret`/`MysqlPrefix`/`CachePrefix`）、`controller/v1/{admin,user}.go`、`models/dao/*`（除 schema 外）、`models/valid`、`internal/task`、`internal/httpmd/jwt_auth.go`、`pkg/{excel,helper,sth_job}`、`sql/user.sql`、`api-test.rest`、`.vscode/launch.json`、`conf/worker.yaml.example` 的删除，以及 `readme.md` → `README.md` 的改名。

依赖按增量方式合并：`go get` 只加 franz-go、kmsg、prometheus/client_golang，`x/sync` 由 indirect 提升为直接依赖，`klauspost/compress` 随 franz-go 升至 v1.20.3；本仓库既有的 rmq、excelize、jwt、aliyun-sls 等依赖全部保留。

**决策 3：SDK 模块路径改为 `serv_tpl/sdk`。**

模板仓库用 `gitee.com/chengduzhihuige/go-serv-template/sdk`，本仓库 origin 不同，且当前 SDK 只在仓库内使用。改为 `serv_tpl/sdk` 与根模块一致，根 go.mod 用 `replace => ./sdk`。代价是该路径无域名，不经远程模块代理分发；对外发布需拆独立仓库，流程写在 [SDK 说明](../../sdk/README.md)。

**决策 4：协议源与生成产物分离。**

`api/` 只留 `.proto`，生成的 Go 代码落在 `sdk/version/v1`。改 `go_package` 后必须重新生成，避免 descriptor 内嵌路径与协议不一致；已验证重新生成幂等。

**决策 5：移植时修正生成脚本的 Windows 兼容性。**

`generate-sdk.ps1` 原版使用 `[IO.Path]::GetRelativePath`（.NET Core 起提供）且为无 BOM 的 UTF-8，在 Windows PowerShell 5.1 下分别导致 API 缺失与中文注释按 ANSI 解码后的语法错误。改为字符串截取取相对路径，并补 UTF-8 BOM；`sdk/README.md` 相应改为「PowerShell 5.1 及以上或 PowerShell 7」。

**决策 6：归档文档按现仓库事实加注记，不篡改原正文。**

D25 正文的 module 路径保留模板仓库原貌，文末追加一条归档注记说明迁入后的变化；`docs/history/` 中 D21 归档文件补编号前缀，与 D22/D24/D25 命名一致。

## 状态勾点（关键里程碑）

- [x] 建立 git 血缘并创建移植分支，工作区零改动
- [x] 移植 `pkg/kafka` 并合并依赖，包内测试通过
- [x] 移植独立 SDK 模块与生成脚本，模块路径本地化，重新生成 pb 并验证幂等
- [x] 装配 routes/controller/bootstrap 的 import、删除旧 pb、补 Dockerfile 与根 go.mod
- [x] 文档规范演进（设计文档编号命名）、归档 D22/D24/D25、重命名 D21 归档
- [x] 全量构建、vet、`go test -race` 与覆盖率核对
- [x] 用户验收

## 验收结论

实现验证（2026-11-06）：

- `go build ./...`、`go vet ./...`、`go mod tidy` 通过，无残留差异。
- `go test ./...` 中本次涉及包全部通过：`pkg/kafka`、`internal/bootstrap`、`routes`、`test/grpc_version`；独立客户端端到端用例真实建临时模块并完成 TCP 调用。
- 存量失败两处，与本改动无关，在 master 的临时 worktree 复现一致：`models/valid Test_JWTParse`、`models/dao/slscal Test_UpSLSData`。
- `go test -race` 未执行：本机无 C 编译器，Windows 下 `-race` 需 cgo，与 [readme](../../readme.md) 中「竞态验证在具备 C 编译器的 Linux 环境执行」的约定一致，未纳入本次结论。
- 覆盖率：`pkg/kafka` 94.5%、`pkg/logger` 91.0%、`pkg/excel` 91.1%、`routes` 100%；`internal/bootstrap` 82.1% 与改动前基线一致，未因本次变更下降。
- 重新执行 `scripts/generate-sdk.ps1` 无内容变化；`git status` 仅体现本次预期改动。

用户验收（2026-11-06）：确认合并范围（只移植有效增量）、血缘策略与 SDK 模块路径三项决策，接受合并结果，本迭代归档。

遗留事项：`sdk` 未对外发布，远程 `go get` 不可用；后续拆分为独立仓库时需同步修改 module 路径、所有 `go_package`、生成脚本与根模块 require/replace。
