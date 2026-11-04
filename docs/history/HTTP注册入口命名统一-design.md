<!-- D15 | 状态：已完成 -->
# HTTP 注册入口命名统一

## 问题

用户要求 HTTP 注册入口命名为 RegisterHTTP，与 RegisterGRPC 保持一致。

## 规则与约束

遵循 [仓库协议](../../AGENTS.md) 与 [项目规范](../../项目规范.md)。

## 方案与决策记录

重命名 HTTP 注册函数及注释，同步启动装配、路由测试、跨协议版本集成测试和接入指南，不保留旧名称别名，路由行为不变。

## 状态勾点（关键里程碑）

- [x] 重命名及文档同步。
- [x] 测试、静态检查与构建。

## 验收结论

2026-11-04：go test ./routes ./internal/bootstrap -run 'TestRegister|TestHTTP|TestGRPC' -count=1、go vet ./routes ./internal/bootstrap、go build ./cmd/... 均通过。首次测试受缓存权限限制，授权后复跑通过。未运行竞态检测，环境 CGO 限制保留。
