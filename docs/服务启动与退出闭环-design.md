<!-- D02 | 状态：进行中 -->
# 服务启动与退出闭环

## 问题

用户已认可方案并授权实施。目标是在保留现有分层和依赖的前提下，形成可启动、失败可见、请求可排空、资源可回收的 HTTP 服务基线。

立项时问题证据（下列函数名描述实现前状态，链接随文件迁移更新）：

| 位置 | 当前问题 |
| --- | --- |
| [HTTP 入口](../cmd/api/main.go) `main` | 没有调用路由注册，丢弃 HTTP 关闭函数；退出时先关闭日志 |
| [启动装配](../internal/bootstrap/bootstrap.go) `MustHttp` | 使用 `gin.Engine.Run`，监听失败仅记日志；关闭回调为空 |
| 同上 `MustInitDBs` | Redis 初始化失败未回收 MySQL，关闭错误被忽略 |
| 同上 `Heartbeat` | recover 位于父函数，子 goroutine 无停止与等待机制 |
| [配置加载](../internal/global/conf.go) 与 [配置模板](../conf/app.yaml.example) | HTTP 端口无默认值且模板缺项，读取为 0 后监听随机端口；缺少 HTTP 超时配置 |
| [路由](../routes/router.go) 与 [健康控制器](../controller/healthz.go) | Ready 恒成功；未实现的 ReloadConf 返回成功，注册路由后会暴露假能力 |
| [MySQL](../models/dao/mysql.go)、[Redis](../models/dao/redis.go) 构造函数 | 初始化未形成明确超时与失败清理契约 |

## 规则与约束

- 工程与分层决策以[项目规范](../项目规范.md)第 1、2、5、6、7、8、9 节为准。
- 登记、状态与归档按 [AGENTS.md](../AGENTS.md)执行，文档结构采用[设计文档模板](设计文档模板.md)。
- 当前状态入口为 [curr.md](../curr.md)，计划登记为 [plan.md](../plan.md)。

## 方案与决策记录

### 1. 范围与阶段

本项进入代码实施与隔离验收阶段，任务可靠性改造保持在范围之外。

实施范围：HTTP 装配、启动失败回滚、服务失败传播、信号与退出编排、心跳生命周期、最小健康语义、必要配置和测试。保持现有 Gin/GORM/Redis/zap/Viper 选型，不引入生命周期框架或新第三方依赖。

边界：

- 不在本项修复 DbJob/RmqJob 的互斥、ACK、幂等与通道协议；不将 worker 数据可靠性计入本项验收结论。
- worker 是纯后台任务进程，不启动 HTTP、不注册路由，也不读取或校验 http.*；使用日志最后关闭的顺序。现有 task.Close 仍可能失败，必须报告而非判定正常退出。
- 不在本项扩展 Prometheus、完整 Trace、环境变量覆盖、通用响应错误体系、CI/Dockerfile、示例任务 DDL 或测试目录整体改造。
- 不新增无存储模式；真实启动仍需要有效 MySQL/Redis 配置。组件单测通过注入替身避免依赖真实存储。
- 当前 ReqLog 的全量请求内容记录存在风险，本项默认入口不挂载该中间件；安全访问日志留待后续专项。本项新增生命周期/恢复日志只记录安全字段，不记录 Body、Header 或原始 DSN。

### 2. 入口与依赖装配

将 `cmd/main.go` 移至 `cmd/api/main.go`，统一每个可执行文件一个子目录。用户启动命令随之变为 `go run ./cmd/api -config conf/app.yaml`；worker 命令不变。入口只调用装配和运行逻辑，业务目录不迁移。

启动顺序：

```text
注册信号监听（覆盖初始化阶段）
→ 读取并校验配置
→ MustInitLog 装载唯一全局日志，立即登记清理
→ 初始化 MySQL、Redis（失败回滚），成功后发布全局连接
→ 启动可选心跳并登记关闭
→ 创建 Gin、注册 Recovery 与路由
→ 同步 net.Listen 成功
→ 启动 Serve，发布就绪状态
→ 等待信号或服务致命错误
```

`routes.Register` 由 cmd 调用，bootstrap 不反向依赖 routes/controller。Gin mode 在创建 engine 前设置。路由和中间件必须全部注册完再启动 Serve，禁止启动后修改路由。

保留规范要求的 `MustInitLog` 装载入口；底层工厂继续返回 error。后续初始化优先返回 error。入口运行边界捕获遗留 Must 类初始化 panic，写安全上下文与堆栈并进入同一清理路径，最终非零退出。已创建资源立即登记清理，不能等到所有初始化成功才登记。

配置/日志装载前的故障使用 `logger.L()` 的兜底实例输出安全诊断。不得将未经筛选的 panic 值、DSN 或配置全文带入日志。

### 3. 生命周期 API

```go
// Cleanup 表示一个有名称、可报告失败的清理步骤。
type Cleanup struct {
    Name string
    Stop func(context.Context) error
}

// HTTPService 持有 server/listener 及 Serve 完成结果。
// StartHTTP 同步监听；失败时不留下后台 goroutine。
func StartHTTP(handler http.Handler, opts HTTPOptions) (*HTTPService, error)
func (s *HTTPService) Done() <-chan error
func (s *HTTPService) Shutdown(ctx context.Context) error

// StartHeartbeat 返回可取消且可等待的关闭函数；关闭时不再启动新采样。
func StartHeartbeat(opts HeartbeatOptions) (func(context.Context) error, error)
```

替换无实际关闭能力的 `MustHttp`，不为其保留空壳兼容函数。运行编排只监听有限的服务结果与信号，使用标准库 context/channel/等待机制即可；不为固定数量的 goroutine 新增 errgroup 依赖。自建 goroutine 均在自身边界 recover，记录组件名和堆栈，致命异常反馈主流程，禁止恢复后静默失去服务。

### 4. HTTP 生命周期与退出码

- 用 `http.Server` 持有 handler 与显式超时，先同步 `net.Listen`，再异步 `Serve`。测试可注入 `127.0.0.1:0` listener；业务配置禁止端口 0。
- 监听失败立即返回；Serve 异常通过容量为 1 的结果通道传回运行编排。只有已进入关闭流程后的 `http.ErrServerClosed` 才视为正常，不掩盖意外终止。
- signal、Serve 错误、初始化失败统一触发一次清理。重复触发不重复 Close；组件 Shutdown 支持幂等。
- 首个退出信号先置不就绪，再调用 Shutdown 停接入并等待活跃请求；请求 context 不直接绑定首个信号即取消的进程 context，否则无法排空正常请求。
- 正常路径：HTTP 排空 → 停止并等待心跳 → 关闭 Redis → 关闭 MySQL → 日志 Sync/Close。worker 适配路径在存储关闭前调用现有任务停止函数，明确其失败风险。
- Shutdown 的排空子期限到达时调用 `Server.Close` 强制断开连接，并取消仍在运行请求的服务级 context；已记录失败，不能返回退出码 0。非合作 handler 不能靠关闭 socket 保证结束，最终由进程总期限兜底。
- 各清理步骤捕获 panic、记录失败并尽量继续后续清理；普通 error 同样使退出失败。主入口负责最终 `os.Exit`。
- 日志清理同样受总期限约束；看门狗不能先执行可能阻塞的 Sync 再强退。
- 首次退出信号后，独立看门狗在总期限到达或收到第二次退出信号时强退 1。正常清理成功时取消看门狗并注销信号订阅。
- 退出码：帮助/正常运行后完成清理为 0；配置、初始化、监听、运行致命错误、清理错误/panic、超时或二次信号为 1。

### 5. 初始化回滚与显式超时

基础设施在局部变量中完成初始化，全部成功后再发布到 `global`；清理闭包捕获局部实例而非重新读取可变全局。Redis 构造或 Ping 失败必须关闭本次客户端，并回滚先前 MySQL；MySQL 任何取得池后的失败均关闭池。

DAO 增加 context 接入，由 bootstrap 传入初始化期限；显式设置驱动 dial/read/write timeout 和 Redis dial/read/write/pool timeout。MySQL 初始化中的版本探测和自动 Ping 也必须落在有限网络等待内，不能只给最后的 PingContext 加期限；实施时通过已有驱动配置或显式构造 sql.DB 控制整条路径。不得用一个超时返回但后台持续建连的 goroutine 冒充取消。

超时与信号取消返回后，禁止继续发布连接或启动 HTTP。启动错误只记录组件和安全诊断，不打印 DSN；本项不继续使用已知有缺陷的 DSN 字符串截断作为安全保证。

### 6. 配置契约

以下配置已在模板与读取代码中接通，并有默认值及边界校验测试。

```yaml
http:
  port: 8080
  debug: false
  read_header_timeout: 5s
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s
  drain_timeout: 20s
startup_timeout: 15s
shutdown_timeout: 30 # 保持已有键的秒整数语义
metrics:
  enable: false
  dt: 10 # 秒
```

- 缺省采用上述值；显式端口必须在 1..65535，负时长/零时长或解析失败拒绝启动。
- `shutdown_timeout` 缺省 30 秒；显式非正值由原来的静默回退改为配置错误。`http.drain_timeout` 必须小于总退出期限，为资源回收留出预算。
- MySQL/Redis 网络超时先在初始化 Options 中集中给出有限默认值（建议 dial/read/write 为 3 秒、Redis pool 为 4 秒），初始化总期限由 `startup_timeout` 控制；不增加多组重复配置键。
- stdout 不要求 `log.path`；file 才校验路径并创建目录。HTTP 只读取 `http.debug`，模板注释明确其与顶层 debug 的关系，避免误用。
- 心跳间隔只在开启时校验为正；默认关闭。现有 SysStatInfo 采样阻塞约 5 秒，关闭必须等待当前采样或明确超时失败，不能返回成功后继续写已关闭日志。

### 7. 恢复与健康路由

- 增加统一 Recovery：未写响应时返回安全 JSON 500；已写响应时终止后续处理，不拼接第二个 JSON。日志通过 logger.L() 记录组件/路由模板/堆栈，不输出请求原始内容。
- `/healthz` 保留存活语义；`/ready` 在启动完成且未进入关闭时返回成功，否则 503。状态用注入的并发安全谓词读取，controller 不依赖 bootstrap。
- 本项 Ready 仅表达生命周期就绪，不宣称持续探测 MySQL/Redis 健康；运行中依赖健康检查留待后续。
- 不注册未实现的 `/reload`，避免接通路由时暴露虚假成功能力；本项不实现热加载。

### 8. 实施文件范围

| 文件 | 最小职责 |
| --- | --- |
| `cmd/main.go` → `cmd/api/main.go` | 入口位置统一；完成路由/生命周期装配 |
| `cmd/sth_worker/main.go` | 适配共享 API、失败退出与清理顺序，不改任务算法 |
| `internal/bootstrap/bootstrap.go` | 保留日志/基础设施装配，失败回滚与错误传播 |
| 新增 `internal/bootstrap/http.go`、`lifecycle.go`、`heartbeat.go` | 将相应已有职责拆出并补齐关闭，不增目录层级 |
| 新增 `internal/global/lifecycle_conf.go` | 新键默认值及严格类型、范围校验；原有配置读取保持兼容 |
| `models/dao/mysql.go`、`redis.go` | context、连接期限和失败清理 |
| 新增 `internal/httpmd/recovery.go` | 安全恢复中间件 |
| `routes/router.go`、`controller/healthz.go` | 生命周期就绪状态注入，不注册假重载 |
| `pkg/logger/logger.go` | 可取消系统采样；传播文件 Sync/Close 错误 |
| `go.mod` | 已存在 MySQL driver 标记为直接依赖，不新增版本 |
| `conf/app.yaml.example`、`pkg/logger/README.md` | 配置与现有心跳使用说明同步 |
| 上述受影响包的 `*_test.go` | 表驱动、故障注入、真实本地 listener、生命周期测试 |

不修改数据表，不新增 SQL。现有文件内部拆分只服务职责清晰，不调整业务分层。与本项无关的文档错误不顺手扩大修复范围。

### 9. 验收标准

验收标准如下，实际执行情况见文末验收结论；未将存量失败与跳过项视为通过。

| 编号 | 场景 | 通过条件 |
| --- | --- | --- |
| A1 | 默认与非法配置 | 缺省端口为 8080；显式 0/越界端口、非法时长失败；stdout 缺 path 可启动，file 缺 path 失败 |
| A2 | HTTP 装配 | 本地实际 listener 上 `/healthz` 200；响应符合现有成功结构；`/reload` 不返回重载成功；启动前路由已完整注册 |
| A3 | 启动/运行失败 | 占用端口导致启动失败且不遗留 Serve/心跳；注入 Serve 错误/panic触发清理，进程非零退出 |
| A4 | 初始化失败与取消 | Redis 初始化失败回收 MySQL；已分配的失败客户端被关闭；启动取消后不发布连接、不启动 HTTP；失败清理每项只执行一次 |
| A5 | 正常排空 | 用受控阻塞 handler 发起请求，再触发退出；停止新接入，在释放 handler 前不关闭存储；释放后响应完整、按序清理、退出 0 |
| A6 | 总期限与二次信号 | handler/清理/日志 Sync 卡住时，在配置总期限加测试调度容差内退出 1；二次信号立即走强退；不能被 Sync 阻塞 |
| A7 | 清理错误和 panic | 单步失败可观察、有组件名；可执行的后续清理仍执行；日志最后关闭；退出 1 |
| A8 | 心跳 | 禁用时无 goroutine；启用后可停止并等待，关闭后无后续采样/日志；子 goroutine panic 被捕获并上报 |
| A9 | Recovery/Ready | handler panic 返回安全 500且后续请求仍可服务；无敏感内容；Ready 随启动/退出切换，结束前不误报就绪 |
| A10 | 重复调用与竞态 | 重复 Shutdown/取消不 panic、不重复释放；并发就绪读取及关闭在 race 下通过 |
| A11 | 真实进程 | Linux 下 SIGINT/SIGTERM 触发排空、二次信号强退；Windows 做可支持的中断与子进程退出码验证，不能用 Windows 结果代替 Linux SIGTERM |
| A12 | 存储集成 | 使用专用临时 MySQL/Redis 验证连接成功、不可达、拒绝认证、失败回收与显式超时；不使用开发者默认数据实例 |

自动化优先使用 httptest、本地随机 listener、注入构造/清理替身和已有测试依赖；真实连接与进程信号用隔离集成环境。时间测试使用同步通道控制状态，避免依赖随意 sleep。

### 10. 验证命令、基线与验收限制

实施时先执行 build/vet、受影响包单测、局部覆盖率与 race，再在隔离环境执行全量回归。核心覆盖率目标按项目规范；不得把局部覆盖率达到 85% 描述成全仓达标。

上轮评审已识别的基线条件：默认 task 测试向真实 Redis 投递大量消息、MySQL 测试缺少有效配置；本机 CGO 关闭且 staticcheck 未安装；当前仓库无有效 HEAD，不能直接创建 HEAD 基线 worktree。这些不是本项已修复的问题。

不得在当前默认环境盲目重跑全量测试。实施前需在隔离副本/专用依赖环境形成可重复基线，给全量测试加明确 timeout；构建测试所需 CGO/staticcheck 环境后再验收竞态与静态检查。无法完成的检查逐项报告为未验证，按规范处理存量失败，不能用跳过冒充通过。

本项涉及基础启动组件，按高风险变更安排全量回归及人工确认。自动化和容器运行验证已执行；用户人工确认尚未进行，暂不归档。

### 11. 编号依据

当前非归档文档唯一可见已用编号为项目规范引用的 D01。用户已明确授权仅核对归档文档首行编号；核对时 `docs/history/` 未发现 Markdown 文件，未读取历史正文。因此本项使用 D02。编号一经分配不复用，后续发现外部未同步记录时先对齐登记，不能静默重排已有编号。

### 12. 实施决策补充

- 两个 main 显式列出配置、日志、存储、心跳及各自的 HTTP/任务启动步骤，不再通过 Prepare/PrepareWorker 隐藏装配清单。保留 Run 外层边界，在初始化前监听信号并统一处理 panic、启动失败回滚和退出；资源创建后立即登记清理，不能延迟到 WaitingExit 才接管。删除无生产调用的 Prepare/PrepareWorker 及 prepare.go，不再保留第二套启动路径。两个 main.go 内将原匿名回调原样命名为 start，main 仍调用 bootstrap.Run(start)，步骤、登记顺序与退出机制不变。相邻 main_test.go 直接测试 start + Run；cmd 的生产代码仍只在 main.go，测试文件不承载生产装配。为入口可读性接受两处短装配代码重复，不新增生命周期 API。心跳在 HTTP 之后、存储之前关闭，监听失败也会停止心跳。
- 用户明确 worker 不启动 HTTP 后，移除共享配置中的隐式 HTTP 校验耦合：缺失或无效的 http.* 不影响 worker，worker 的 shutdown_timeout 也不受 HTTP drain_timeout 限制。API 的 HTTP 校验保持不变。
- 入口测试替换原 prepare_test.go：API 覆盖非法 HTTP/共享配置、配置读取 panic、已取消启动、无效 DSN；worker 覆盖缺失/非法 HTTP 不阻断共享装配、非法共享配置拒绝、取消与存储配置失败。失败路径检查退出码及已创建日志 writer 的关闭。API 隔离集成使用显式 D02_TEST_MYSQL_DSN 和 miniredis，覆盖真实健康路由、主动故障退出、端口冲突后 HTTP/存储/日志释放。worker 不运行存在已知可靠性问题的真实任务；任务成功路径仍不计入本项验收。既有生命周期测试继续覆盖正常信号、清理顺序、panic 和超时看门狗，不修改基础组件以迁就测试。
- startup_timeout 限定存储初始化总期限；驱动网络超时显式为 3 秒，Redis 池等待为 4 秒。配置读取与日志文件创建是同步本地操作；初始化阶段收到信号同样启动进程退出看门狗。
- MySQL 使用现有驱动 NewConnector → sql.OpenDB → PingContext，GORM 关闭重复自动 Ping 及无 context 的版本探测。本次用 MySQL 8.0.36 验证；其他数据库版本尚未验证。原有 OpenMysql/ConnMysql/OpenRedis 入口保留为有界兼容包装。
- 初始化日志不输出原始 DSN/密码；DAO 返回安全阶段错误。GORM 参数化日志开启；ZapWriter 对可变参数长度做保护。没有更改任务业务日志策略。
- pkg/logger 增加 SysStatInfoContext，使生产心跳采样可取消；Close 传播文件 Sync/Close 错误。stdout 为未缓冲写入，不关闭 stdout。日志故障和看门狗强退以退出码报告，强退路径不依赖日志写入完成。
- 退出编排仍用标准库，没有新增依赖版本；go.mod 仅把已存在且本次直接使用的 MySQL driver 从 indirect 改为直接依赖。
- 在没有 HEAD 的情况下，变更前代码保存到本机临时基线副本；没有创建提交或使用 stash。该副本与当前源码都挂载为只读，在专用容器执行同样检查。
- 验证期间检测到外部编辑在 internal/task/sth_mq_task_test.go 增加 SkipNow 并调整循环，予以保留，未纳入本项代码改造；该用例当前计为跳过，绝不计为通过。

## 状态勾点（关键里程碑）

- [x] 用户指定启动本项，限定本轮仅设计登记。
- [x] 核对当前实现、协议与编号，登记最小方案和验收标准草稿。
- [x] 用户审阅方案，确定进入代码实施。
- [x] 建立隔离验证基线，落实初始化与退出接口。
- [x] 完成 HTTP 装配、失败回滚、心跳与健康路由。
- [x] 完成自动化验证与全量回归，区分并报告存量失败。
- [ ] 用户完成高风险变更人工验收。
- [ ] 按协议收尾并归档。

## 验收结论

自动化验收日期：2026-11-03（Asia/Shanghai）。

结论：本项实现与隔离自动化验收完成，未发现本次新增检查失败；全仓检查仍有下表列出的存量失败。高风险变更的用户人工确认未完成，因此保持进行中，不执行最终归档。

| 验证 | 结果 |
| --- | --- |
| Windows/Linux Go 1.26.10，go build ./...、go vet ./... | 通过 |
| 受影响包单测及真实隔离 MySQL 8.0.36 / Redis 7 集成 | 通过；覆盖正常连接、认证失败、关闭、构造失败回滚、模拟静默握手超时 |
| Linux CGO：受影响包 go test -race -count=1 -timeout=60s，带显式隔离实例参数 | 通过，无竞态报告 |
| A1–A10 | 自动化通过：配置、路由、监听失败、Serve 故障、回滚、排空、强制取消、看门狗、幂等关闭、Recovery、就绪切换 |
| A11 Linux 真实子进程 | SIGINT/SIGTERM 正常排空退出 0；卡住清理总期限退出 1；二次信号强退 1；Windows 验证失败子进程退出码，信号测试明确跳过 |
| 真实 API 容器 | /healthz 与 /ready 返回成功；同容器第二实例占用端口退出 1；原服务 SIGTERM 退出 0 |
| A12 存储集成 | 专用容器、独立数据库与随机宿主端口，不连接开发者默认 MySQL/Redis；通过 |
| Linux go test -count=1 -timeout=45s ./... | internal/task 的 Test_GetTasks 因默认数据库依赖缺失失败；其余已执行包通过，Test_AddTasks 为外部编辑造成的跳过 |
| 变更前基线全量与定向复跑 | 默认 Redis 测试连接拒绝；定向 MySQL 测试同样连接拒绝，确认属于原有依赖问题 |
| Linux go test -race -count=1 -timeout=45s ./... | 同样仅任务包因存量依赖失败；其余已执行包通过，无竞态报告；不能表述为全量 race 通过 |
| staticcheck 2026.2.2 (0.8.2) ./... | 当前与基线均仅 routes/admin.go:7 的 registAdmin 未使用（U1000），本项不顺手删除空占位 |
| 受影响模块联合覆盖率 | 92.1%，包含 bootstrap/global/dao/logger/httpmd/controller/routes；Recovery 与健康路由均 100%，未剔除旧 ReqLog |
| 全仓核心覆盖率 | 未声明达标；任务等未改模块的低覆盖率及全量测试失败仍保留 |

worker 配置解耦补充验证：`go test -count=1 -timeout=30s ./internal/global ./internal/bootstrap`、`go build ./cmd/...`、`go vet ./internal/global ./internal/bootstrap ./cmd/...` 均通过。新增用例验证 worker 忽略缺失或非法 HTTP 配置、仍拒绝非法共享配置，以及当时 PrepareWorker 的装配路径（该接口现已删除，测试已迁至 worker 的 start）；使用临时配置与已取消的 context，不连接外部存储。构建与 vet 首次因沙箱无权访问 Go 缓存失败，授权重跑通过。上表联合覆盖率为此前隔离验收结果，本次补充未重测覆盖率。

关键复现命令（依赖参数必须来自专用测试实例，不提供生产默认值）：

入口显式装配补充验证：`go test -count=1 -timeout=30s ./internal/global ./internal/bootstrap ./routes`、`go build ./...`、`go vet ./...` 均通过。此次仅展开入口装配，未修改生命周期基础组件或任务实现；复用现有配置、回滚、清理、HTTP 与路由测试。未设置真实存储集成参数，对应用例明确跳过；未重跑容器全量回归、race 或覆盖率，上表仍为此前隔离验收记录。构建和 vet 首次受 Go 缓存访问权限限制，授权重跑通过。

删除组合入口后的补充验收：

- Windows：`go test -count=1 -timeout=45s ./cmd/... ./internal/bootstrap ./internal/global ./routes`、`go build ./...`、`go vet ./...` 通过。测试使用专用 MySQL 8.0.36 容器及 miniredis，API 的 TestStartIsolated 实际执行，非跳过；未连接开发者默认存储。
- Linux Go 1.26 容器：相同包执行 `go test -race -count=1 -timeout=60s -coverprofile=/tmp/entry.out ./cmd/... ./internal/bootstrap ./internal/global ./routes` 通过，无竞态报告，包含既有真实子进程 SIGINT/SIGTERM、二次信号与超时测试。
- 本轮联合覆盖率 87.7%；分包 API 78.4%、worker 39.4%、bootstrap 92.8%、global 98.7%、routes 100%。入口分包未达到 85%，主要未覆盖 CLI 分支及 worker 真实任务成功路径；联合结果不代表每包或全仓核心覆盖率达标。原 prepare_test.go 已删除，不能再将旧组合接口覆盖率算作入口验证。
- 验证初期修正了测试自身的两个假设：切割日志 writer 无 Sync 方法，改为关闭后直接写入检查；Windows 端口冲突测试改用与服务相同的通配监听地址。最终上述测试全部通过，生产生命周期实现未修改。
- Go 缓存和 Docker 初次受沙箱权限限制，授权后验证通过。专用 gtank-d02-entry-mysql 容器与匿名卷已清理，Go 验证容器自动删除。本轮未重跑全仓测试与 staticcheck；其存量问题仍以上表为准。

```bash
go build ./...
go vet ./...
go test -race -count=1 -timeout=60s \
  -coverprofile=/tmp/d02-affected.out \
  -coverpkg=./internal/bootstrap,./internal/global,./models/dao,./pkg/logger,./internal/httpmd,./controller,./routes \
  ./internal/bootstrap ./internal/global ./models/dao ./pkg/logger ./routes
go tool cover -func=/tmp/d02-affected.out
go test -count=1 -timeout=45s ./...
go test -race -count=1 -timeout=45s ./...
staticcheck ./...
```

受影响模块集成命令需设置 D02_TEST_MYSQL_DSN、D02_TEST_MYSQL_ADDR、D02_TEST_REDIS_ADDR；未设置时显式跳过存储集成用例。全量命令只能在隔离环境运行，默认任务测试尚未整体拆分。

遗留边界：任务锁/ACK/幂等/Close 缺陷未改；Redis 认证失败由 miniredis 用例覆盖；Ready 只表示生命周期就绪；Trace/访问日志/Prometheus 未在本项实现；DsnMask 旧工具仍存在缺陷，但新启动路径不调用它；完整模板复制验收不能仅凭本项通过而宣布就绪。

测试资源收尾：仅本次创建的 gtank-d02-* 测试容器、匿名卷及 gtank-d02-review 网络已清理；联合覆盖率原始文件保留在仓库忽略目录 tmp/d02-affected.out。下载的通用 Go 镜像保留在本机缓存，不影响运行服务。

待用户人工确认：启动命令迁移、配置契约、Ready 语义与停机行为。确认后按 AGENTS 收尾顺序更新计划与现状并归档本设计。
