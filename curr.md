# 当前现状

- HTTP 路由与 gRPC 业务服务分别在 routes.RegisterHTTP、routes.RegisterGRPC 注册；bootstrap 在监听前调用对应入口，并管理标准 gRPC Health 服务及服务器生命周期。

- 用户控制器是未注册的 demo：不检查身份，创建仅校验并返回 demo/validated；列表通过 global.MysqlCli 查询 users 表，当前只应用手机号筛选，并直接返回 schema.User。接入步骤见 [业务接入指南](docs/业务接入指南.md)。

- SuccJsonRaw 保留原始 JSON 透传，成功码为 0，request_id 经 JSON 转义；data 合法性由调用方保证。

- HTTP Response/Fail 识别错误链中的 *Exception 和 Exception，保留业务状态码、错误码及响应内容；nil error 和匹配到的空 *Exception 返回通用 500 错误；dev 普通错误保留完整原始消息，非 dev 使用通用消息。

- API 入口为 `cmd/api`，步骤4 顺序调用 MustStartHTTP(cfg)、MustGrpcServ(cfg)；gRPC 默认关闭，开启后提供标准 Health RPC 和 VersionService.Version 版本查询，与 HTTP 根路径返回相同的 name/desc/version。命令行客户端为 test/grpc_version，使用方式见 [gRPC 版本查询](docs/gRPC版本查询.md)。HTTP/gRPC 独立监听，均支持运行故障通知与超时排空，存储及日志最后关闭。
- `cmd/sth_worker` 是待接入业务的空入口，仅配置、日志、可选心跳和等待退出；不初始化存储、不启动 HTTP/gRPC、不校验两种协议配置。internal/task 已删除，pkg/sth_job 仍保留且无入口引用。
- 两个 main.go 直接顺序执行 Must 启动；失败直接 panic，不做统一恢复或跨组件回滚。初始化存储调用有超时，业务补偿归业务层。
- 心跳开关在 bootstrap.Heartbeat 内判断，无返回值，两个入口各保留一行调用；采样错误仅记录，panic 在自身 goroutine 内恢复，不参与 WaitingExit，随进程退出结束。
- WaitingExit 显式接收 HTTP/gRPC 两个结果通道，普通 select 等待信号或服务停止，nil 通道不参与等待；Cleanup 仅负责关闭。closeResource 逐项隔离清理异常，按入口顺序关闭，保留总期限/二次信号强退。

# 已知问题

- 当前环境未启用 CGO，无法执行竞态检测。

- main 包快速失败测试在普通模式和 -short 下均执行；API 完整入口集成由 -short 跳过，非短模式须显式提供隔离 MySQL 参数，Redis 使用测试内 miniredis。
- 全仓覆盖率尚低于规范 85% 门槛；gRPC 尚无生产业务 RPC、TLS、鉴权或反射。

# 下一步

- 暂无进行中的迭代。
