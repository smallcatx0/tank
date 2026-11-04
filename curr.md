# 当前现状

- HTTP 入口为 `cmd/api`，步骤4 仅调用 MustStartHTTP(cfg)，内部完成 Gin/路由装配、直接构造地址监听及就绪发布；包内 httpOptions 仅承载阶段期限。Shutdown 内部撤销就绪并排空请求，运行后日志最后关闭。
- `cmd/sth_worker` 仅运行后台任务，不启动 HTTP，也不校验 HTTP 配置。
- 两个 main.go 直接顺序执行 Must 启动；失败直接 panic，不做统一恢复或跨组件回滚。初始化存储调用有超时，业务补偿归业务层。
- 心跳开关在 bootstrap.Heartbeat 内判断，无返回值，两个入口各保留一行调用；采样错误仅记录，panic 在自身 goroutine 内恢复，不参与 WaitingExit，随进程退出结束。
- WaitingExit 在单个函数内注册信号、等待信号或 HTTP 停止并执行退出编排，closeResource 负责逐项隔离清理异常，按入口列出的顺序关闭资源，保留日志最后关闭及总期限/二次信号强退。
- [D02 服务启动与退出闭环](docs/服务启动与退出闭环-design.md)处于人工确认阶段；最新心跳、退出实现合并及 HTTP 装配简化未运行测试、构建或静态检查。

# 已知问题

- 检查结果、存量失败与本项边界见 [D02 验收结论](docs/服务启动与退出闭环-design.md#验收结论)。

# 下一步

- 人工确认 [D02 验收结论](docs/服务启动与退出闭环-design.md#验收结论)列出的运行契约；确认后按协议收尾归档。
