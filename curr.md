# 当前现状

## 架构与能力

- 根模块 `serv_tpl`（Go 1.26）；分层与目录职责见 [项目规范](项目规范.md) 第 9 节。
- 双入口：`cmd/api` 装配 HTTP（Gin）与可选 gRPC，`cmd/sth_worker` 只跑任务、不启 HTTP/gRPC。
- 协议与 SDK 分离：`api/` 只存 `.proto`；生成的 Go 消息与 gRPC 接口在子模块 `sdk/`（`serv_tpl/sdk`）。根模块以 `require serv_tpl/sdk v0.0.0` + `replace => ./sdk` 引用，Dockerfile 在 `go mod download` 前复制 `sdk/go.mod`、`sdk/go.sum`。
- 客户端通用能力：`pkg/kafka`（franz-go 消费封装，含错误分类、Prometheus 指标、故障注入测试）、`pkg/logger`、`pkg/excel`、`pkg/helper`、`pkg/sth_job`。
- 协议生成：`scripts/generate-sdk.ps1`（PowerShell 5.1 及以上，文件带 UTF-8 BOM）与 `scripts/generate-sdk.sh`，锁定 protoc 33.0、protoc-gen-go v1.36.12、protoc-gen-go-grpc 1.6.0，重复执行结果一致。
- 设计文档命名为 `docs/编号-需求名称-design.md`；已完结迭代归档在 `docs/history/`，当前最大编号 D26。

## 已知问题

- `sdk` 模块路径 `serv_tpl/sdk` 无域名，只能通过本地 replace 使用，不经远程模块代理分发；对外发布需拆独立仓库。
- 执行策略为 Restricted 的 Windows 环境需以 `-ExecutionPolicy Bypass` 调用 `scripts/generate-sdk.ps1`。
- 存量失败：`models/valid Test_JWTParse`、`models/dao/slscal Test_UpSLSData`，与本次改动无关。
- `go test -race` 需 C 编译器，本机未安装时只能在具备 gcc 的 Linux 环境执行。

## 下一步

- 无进行中迭代。候选方向见 [plan.md](plan.md)。
