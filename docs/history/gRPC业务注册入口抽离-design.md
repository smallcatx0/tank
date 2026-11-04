<!-- D14 | 状态：已完成 -->
# gRPC 业务注册入口抽离

## 问题

用户要求将频繁变动的业务 RPC 注册从启动装配中抽离，参照 HTTP 路由定义。

## 规则与约束

遵循 [仓库协议](../../AGENTS.md) 与 [项目规范](../../项目规范.md)。

## 方案与决策记录

- 新增 routes/grpc.go 的 RegisterGRPC(grpc.ServiceRegistrar)，集中注册业务服务。
- MustGrpcServ 在监听前调用该入口，不再依赖业务协议与控制器；服务器构造、拦截器、监听和关闭仍由 bootstrap 负责。
- 标准 Health 服务与排空状态关联，保留在 bootstrap 注册及管理。
- 增加注册入口测试，复用已有真实 RPC 往返、Health、关闭和恢复集成测试；同步两份接入文档。

## 状态勾点（关键里程碑）

- [x] 代码与文档调整。
- [x] 注册测试、集成回归及构建检查。

## 验收结论

2026-11-04：go test ./routes ./internal/bootstrap -run 'TestRegisterGRPC|TestRoutes|TestGRPC' -count=1 通过，包含业务注册、真实版本 RPC 往返、Health、排空及恢复测试；go vet ./routes ./internal/bootstrap 与 go build ./cmd/... 通过。静态检查首次受缓存权限限制，授权复跑通过。未运行竞态检测，环境 CGO 限制仍保留。
