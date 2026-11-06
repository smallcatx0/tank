<!-- D25 | 状态：已完成 -->
# 独立 Go 客户端 SDK

## 问题

用户决定采用同一仓库下独立 `sdk/` Go module，向 Go 客户端提供可直接 import 的 RPC 契约，统一生成、发布与调用步骤。原生成代码位于根模块 `serv_tpl/api`，无法独立发布，客户端需自行生成。

## 规则与约束

- 遵循 [工程规范](../../项目规范.md) 和 [流程约定](../../AGENTS.md)。
- 使用说明统一维护在 [SDK 说明](../../sdk/README.md)，[业务接入指南](../../业务接入指南.md) 链接引用。

## 方案与决策记录

- 根据现有 origin，模块路径为 `gitee.com/chengduzhihuige/go-serv-template/sdk`，生成包为 `.../sdk/version/v1`。
- `api/` 是唯一协议源，保留协议文件路径和 RPC wire 名称；`go_package` 指向 SDK。生成代码只保留在 SDK，服务端、测试和示例同步迁移 import。
- SDK 仅依赖 gRPC/protobuf 运行库，不依赖根模块或业务配置。生成代码保留生成器英文注释，不手工改写。Go 版本暂与服务端统一为 1.26.0。
- 根模块 require SDK v0.0.0 并 replace 到 ./sdk，用于尚未发布时本地构建；SDK 自身无 replace，不要求 go.work。发布后外部消费者按正式版本获取，不继承根模块 replace。
- 子模块标签采用 `sdk/v0.1.0` 等前缀；首次发布前提交协议、生成文件、模块锁文件和说明。不在此次操作中推送或创建发布标签。
- 地址、凭据、deadline 和连接生命周期由客户端持有，SDK 不新增连接封装。保留可运行客户端示例，支持 flag > GRPC_ADDR > 默认回环地址。
- Docker 依赖下载层必须先复制 SDK 模块文件；根与 SDK 分别执行检查，避免根 ./... 跳过子模块造成漏检。

## 状态勾点（关键里程碑）

- [x] 确认独立模块方案与实际模块路径
- [x] 迁移生成代码、服务端引用和构建入口
- [x] 更新生成、发布、客户端接入说明
- [x] 验证可重复生成、双模块构建和真实 RPC 调用
- [x] 用户验收

## 验收结论

实现验证（2026-11-05）：

- 根模块 `go test ./...`、`go build ./cmd/...`、`go vet ./...` 通过。
- SDK 模块独立 test/build/vet 通过；该模块仅含生成代码，无手写核心逻辑覆盖率门槛。
- 双模块 staticcheck v0.7.0 通过；重复生成文件哈希一致，文档相对文件链接和 `git diff --check` 通过。
- 独立客户端测试仅 require SDK 并 replace 到本地 SDK，通过真实 TCP 调用现有服务；覆盖环境变量地址及命令行优先级，模块图不包含 serv_tpl。客户端非法地址/超时选项测试通过。
- Dockerfile 依赖层已补充 SDK 模块文件；Go 1.26 Linux 容器内 `go test -race ./internal/bootstrap ./routes ./test/grpc_version`、SDK `go test -race ./...`、`CGO_ENABLED=0 go build ./cmd/api` 通过。首次容器检查因校验和服务网络不可达中断，测试改为复用 SDK go.sum 后通过，未关闭校验。

用户验收（2026-11-05）：验收通过，本迭代归档。远程发布、无 replace 的远程 go get 与完整运行镜像构建未执行；这些不属于本次已执行结果，后续按 [SDK 说明](../../sdk/README.md)发布并核验远程获取。

> 归档注记（迁入本仓库时补充）：本仓库 origin 与模板仓库不同，SDK 模块路径改为 `serv_tpl/sdk`，go_package、生成脚本、根模块 require/replace 与调用方 import 同步变更；正文保留当时在模板仓库的决策原貌。
