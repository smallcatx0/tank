<!-- D02 | 状态：已完成 -->
# 服务启动与退出闭环

后续扩展：[D03 API 可选 gRPC 装配](API可选gRPC装配-design.md)扩展服务故障等待契约。下文历史验收记录不代表该扩展的验收结果；任务实现已由用户删除，worker 当前仅保留配置、日志、心跳及退出入口。

## 问题

用户已认可方案并授权实施。目标是在保留现有分层和依赖的前提下，形成可启动、失败可见、请求可排空、资源可回收的 HTTP 服务基线。

立项时问题证据（下列函数名描述实现前状态，链接随文件迁移更新）：

| 位置 | 当前问题 |
| --- | --- |
| [HTTP 入口](../../cmd/api/main.go) `main` | 没有调用路由注册，丢弃 HTTP 关闭函数；退出时先关闭日志 |
| [启动装配](../../internal/bootstrap/bootstrap.go) `MustHttp` | 使用 `gin.Engine.Run`，监听失败仅记日志；关闭回调为空 |
| 同上 `MustInitDBs` | Redis 初始化失败未回收 MySQL，关闭错误被忽略 |
| 同上 `Heartbeat` | recover 位于父函数，子 goroutine 无停止与等待机制 |
| [配置加载](../../internal/global/conf.go) 与 [配置模板](../../conf/app.yaml.example) | HTTP 端口无默认值且模板缺项，读取为 0 后监听随机端口；缺少 HTTP 超时配置 |
| [路由](../../routes/router.go) 与 [健康控制器](../../controller/healthz.go) | Ready 恒成功；未实现的 ReloadConf 返回成功，注册路由后会暴露假能力 |
| [MySQL](../../models/dao/mysql.go)、[Redis](../../models/dao/redis.go) 构造函数 | 初始化未形成明确超时与失败清理契约 |

## 规则与约束

- 工程与分层决策以[项目规范](../../项目规范.md)第 1、2、5、6、7、8、9 节为准。
- 登记、状态与归档按 [AGENTS.md](../../AGENTS.md)执行，文档结构采用[设计文档模板](../设计文档模板.md)。
- 当前状态入口为 [curr.md](../../curr.md)，计划登记为 [plan.md](../../plan.md)。

## 方案与决策记录

### 1. 范围与阶段

本项进入代码实施与隔离验收阶段，任务可靠性改造保持在范围之外。

实施范围：HTTP 装配、启动快速失败、服务失败传播、信号与退出编排、心跳生命周期、最小健康语义、必要配置和测试。保持现有 Gin/GORM/Redis/zap/Viper 选型，不引入生命周期框架或新第三方依赖。

边界：

- 不在本项修复 DbJob/RmqJob 的互斥、ACK、幂等与通道协议；不将 worker 数据可靠性计入本项验收结论。
- worker 是纯后台任务进程，不启动 HTTP、不注册路由，也不读取或校验 http.*；使用日志最后关闭的顺序。现有 task.Close 仍可能失败，必须报告而非判定正常退出。
- 不在本项扩展 Prometheus、完整 Trace、环境变量覆盖、通用响应错误体系、CI/Dockerfile、示例任务 DDL 或测试目录整体改造。
- 不新增无存储模式；真实启动仍需要有效 MySQL/Redis 配置。组件单测通过注入替身避免依赖真实存储。
- 当前 ReqLog 的全量请求内容记录存在风险，本项默认入口不挂载该中间件；安全访问日志留待后续专项。本项新增生命周期/恢复日志只记录安全字段，不记录 Body、Header 或原始 DSN。

### 2. 入口与依赖装配

两个 main.go 采用直线式 Must 启动，不使用 run/start 回调、Runtime/Lifecycle 对象或入口 recover/defer 清理。业务补偿由业务层负责。

```text
解析启动参数 → MustInitAppConf → MustLoad 配置校验
→ MustInitLog → MustInitDBs（有总初始化期限）
→ 仅 metrics.enable=true 时启动可选 Heartbeat
→ API：MustStartHTTP(cfg)（内部装配 Gin/Recovery/路由、监听并发布就绪）
  worker：MustStartSthTask，不启动 HTTP
→ WaitingExit（此时才注册信号监听）→ 按结果 os.Exit
```

初始化失败直接 panic，由 Go 运行时非零退出；已成功的其他组件不做统一回收，不保证此阶段执行日志 Sync/Close。初始化期间收到退出信号按系统默认行为处理，运行期退出看门狗尚未启动。配置读取和日志创建是本地同步操作；MySQL/Redis 连接使用 startup_timeout 和驱动网络超时，不能用启动 panic 代替超时。

按用户要求，MustStartHTTP 统一负责本服务 HTTP 装配，直接引用 routes 和 internal/httpmd；main 不再操作 Gin、路由或就绪状态。此处替代此前 bootstrap 不引用业务路由的决策，也是项目规范中 httpmd 仅由 cmd/routes 装配规则的本次明确例外；依赖保持 cmd → bootstrap → routes/controller/httpmd，不允许业务层反向引用 bootstrap。所有路由在 Serve 前装配。全局日志由 MustInitLog 唯一装载；配置加载、存储连接和监听错误使用安全 panic 消息，不输出完整配置、DSN 或密码。

MustStartHTTP 直接执行 net.Listen，失败沿用安全 panic；删除仅串联监听与启动的 StartHTTP。私有 serveHTTP 保留 HTTP Server、请求 context、Serve goroutine 及故障通知职责；Shutdown 保留就绪撤销与排空职责。测试的端口冲突走 MustStartHTTP，底层排空/取消/故障场景直接向 serveHTTP 提供 listener。

HTTP 阶段期限采用私有 httpOptions，仅在 bootstrap 包内传递；删除原 HTTPOptions.Address，监听地址直接在 MustStartHTTP 中由 cfg.Port 构造。测试使用同一私有类型，listener 仍由各用例显式创建，不改变监听、超时或退出行为。

### 3. 启动与运行后退出 API

```go
func MustInitDBs(timeout time.Duration) func(context.Context) error
func Heartbeat(enabled bool, interval time.Duration)
func MustStartHTTP(cfg global.LifecycleConfig) *HTTPService

type Cleanup struct {
    Name string
    Stop func(context.Context) error
}
func WaitingExit(timeout time.Duration, httpDone, grpcDone <-chan error, steps ...Cleanup) int
```

Must 包装器不捕获 panic。WaitingExit 不接收启动回调、不恢复启动 panic、不执行启动回滚。服务结果由两个显式通道提供；多服务等待规则见 [D03](API可选gRPC装配-design.md)。清理步骤按入口给出的顺序执行，不依赖注册栈或自动排序。

WaitingExit 内直接完成信号注册、事件等待、看门狗及顺序清理，删除私有 waitingExit 和 forceExit 注入参数；强退直接调用 os.Exit(1)，不增加全局替换变量。closeResource 保留单步 panic 隔离边界。测试直接调用公开入口：HTTP 事件及清理顺序在进程内验证，强退使用独立子进程。真实信号测试仅在支持的平台运行；测试专用 goroutine 观察 WaitingExit 已处于 select 等待的运行栈后再输出 READY，避免注册信号前误发信号。栈格式不匹配时明确超时失败，不依赖固定延时或生产测试钩子。

### 4. 运行后优雅退出与退出码

- 首个信号或 HTTP Serve 结果触发退出；心跳不接入退出链路；开始清理前启动总期限 context 和独立看门狗。第二个信号或期限到达强退 1，不等待日志写入或 Sync。
- API 首步调用 HTTPService.Shutdown，内部先撤销 Ready，再停接入并排空请求；请求 context 不绑定首个信号，排空期限到达时取消请求并强制断开。
- API 关闭顺序为 HTTP → Redis/MySQL → 日志。worker 为任务 → Redis/MySQL → 日志。存储内部仍先关闭 Redis 再关闭 MySQL。
- 每个关闭步骤单独隔离 panic/error，尽量继续后续步骤；任何关闭失败均返回 1。运行故障在日志关闭前记录。HTTP Shutdown 会返回 Serve 故障，避免信号与故障并发时误报成功。
- 帮助及运行后正常清理退出 0；运行故障、关闭失败、超时或二次信号退出 1。启动配置/连接/监听失败由未捕获 panic 非零退出，不再承诺启动失败码固定为 1。
- worker 的锁/ACK/幂等/Close 已知问题保持原范围边界，不将本项测试结果视为任务可靠性通过。

### 5. 初始化超时与责任边界

MustInitDBs 为 MySQL/Redis 整条构造链设置 startup_timeout；驱动 dial/read/write 显式为 3 秒，Redis 池等待为 4 秒。成功后发布全局连接并返回运行后关闭函数。

移除装配层 initDBs 的跨组件失败回滚：Redis 构造失败时不主动关闭此前成功的 MySQL，进程退出回收本地资源。DAO 构造函数自身失败的局部资源释放保留，这是组件自身责任，不是统一回滚。事务、业务副作用和分布式补偿由业务层处理。

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
- 心跳间隔只在开启时校验为正；默认关闭。SysStatInfo 采样阻塞约 5 秒，心跳随进程退出结束，不取消或等待采样；用户接受退出期间丢失心跳日志或产生日志写入错误。

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
| `internal/bootstrap/bootstrap.go` | 保留日志/基础设施装配，启动失败 panic 与运行后关闭 |
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
| A3 | 启动/运行失败 | 占用端口直接 panic 非零退出，不进入 WaitingExit；运行期 Serve 错误/panic 触发清理 |
| A4 | 初始化失败与超时 | 初始化超时或错误直接 panic；不执行装配层跨组件回滚；DAO 自身构造失败资源释放仍保留 |
| A5 | 正常排空 | 用受控阻塞 handler 发起请求，再触发退出；停止新接入，在释放 handler 前不关闭存储；释放后响应完整、按序清理、退出 0 |
| A6 | 总期限与二次信号 | handler/清理/日志 Sync 卡住时，在配置总期限加测试调度容差内退出 1；二次信号立即走强退；不能被 Sync 阻塞 |
| A7 | 清理错误和 panic | 单步失败可观察、有组件名；可执行的后续清理仍执行；日志最后关闭；退出 1 |
| A8 | 心跳 | 禁用时无 goroutine；启用后独立采样，错误仅记录；panic 在自身 goroutine 内恢复、记录堆栈并结束心跳，不触发服务退出 |
| A9 | Recovery/Ready | handler panic 返回安全 500且后续请求仍可服务；无敏感内容；Ready 随启动/退出切换，结束前不误报就绪 |
| A10 | 重复调用与竞态 | 重复 Shutdown/取消不 panic、不重复释放；并发就绪读取及关闭在 race 下通过 |
| A11 | 真实进程 | Linux 下 SIGINT/SIGTERM 触发排空、二次信号强退；Windows 做可支持的中断与子进程退出码验证，不能用 Windows 结果代替 Linux SIGTERM |
| A12 | 存储集成 | 使用专用临时 MySQL/Redis 验证连接成功、不可达、拒绝认证、失败回收与显式超时；不使用开发者默认数据实例 |

自动化优先使用 httptest、本地随机 listener、注入构造/清理替身和已有测试依赖；真实连接与进程信号用隔离集成环境。时间测试使用同步通道控制状态，避免依赖随意 sleep。

### 10. 验证命令、基线与验收限制

main 包测试按成本和依赖执行，不整体排除：

- 两个入口的 TestEntryStartupPanic 为快速失败测试，只用临时配置和子进程，不连接外部存储；普通模式及 -short 均执行，保留非零退出、敏感信息不输出和 worker 忽略 HTTP 配置的断言。
- API 的 TestEntryIsolated 涉及完整启动、存储连接、就绪轮询和退出，-short 优先跳过，即使环境变量已设置也不执行；非短模式仍须显式提供 D02_TEST_MYSQL_DSN，缺少时报告跳过。当前该用例使用真实隔离 MySQL + 测试内 miniredis，并非真实 Redis 集成；不新增外部 Redis 依赖。其他包已有真实存储集成开关不变。
- main 包快速检查：go test -short ./cmd/...；普通入口测试：go test ./cmd/...；隔离入口集成：配置专用 D02_TEST_MYSQL_DSN 后执行 go test -count=1 -run '^TestEntryIsolated$' ./cmd/api（不加 -short，正常 SIGTERM 用例在 Linux 执行）。
- 修改启动/退出装配或发布前须运行非短模式和显式隔离集成；短模式通过或集成跳过不能视为全量回归通过。本次不调整其他包的 -short 行为，不能据此保证 go test -short ./... 不访问外部存储。

实施时先执行 build/vet、受影响包单测、局部覆盖率与 race，再在隔离环境执行全量回归。核心覆盖率目标按项目规范；不得把局部覆盖率达到 85% 描述成全仓达标。

上轮评审已识别的基线条件：默认 task 测试向真实 Redis 投递大量消息、MySQL 测试缺少有效配置；本机 CGO 关闭且 staticcheck 未安装；当前仓库无有效 HEAD，不能直接创建 HEAD 基线 worktree。这些不是本项已修复的问题。

不得在当前默认环境盲目重跑全量测试。实施前需在隔离副本/专用依赖环境形成可重复基线，给全量测试加明确 timeout；构建测试所需 CGO/staticcheck 环境后再验收竞态与静态检查。无法完成的检查逐项报告为未验证，按规范处理存量失败，不能用跳过冒充通过。

本项涉及基础启动组件，按高风险变更安排全量回归及人工确认。自动化和容器运行验证已执行；用户人工确认尚未进行，暂不归档。

### 11. 编号依据

当前非归档文档唯一可见已用编号为项目规范引用的 D01。用户已明确授权仅核对归档文档首行编号；核对时 `docs/history/` 未发现 Markdown 文件，未读取历史正文。因此本项使用 D02。编号一经分配不复用，后续发现外部未同步记录时先对齐登记，不能静默重排已有编号。

### 12. 最新边界决策

用户明确启动装配不承担业务补偿。当前方案以第 2–5 节及本节为准；后文较早验收记录中的入口 recover/defer、启动看门狗、统一回滚及心跳停止/故障传播均为当轮实现，不再代表当前行为。按用户明确边界，心跳采用进程级 goroutine，不要求单独取消和等待；这是针对通用 goroutine 可停止约定的明确例外，接受日志关闭附近的心跳日志丢失或写入错误。

- 删除旧 Run/start/Runtime、WatchSignals、Cleanup 函数及入口恢复闭包；Cleanup 现仅为运行后有名称的关闭步骤。
- backgroundDone 已无生产使用方，删除该参数及监听分支，不为潜在需求保留通道。API 传 server.Done()，worker 仅为 httpDone 保留一个 nil；信号、HTTP 停止、清理异常、日志最后关闭、总期限及二次信号强退行为保持不变。此前通用后台故障测试契约不再适用，子进程运行故障用例改用 HTTP 通道。
- 必需组件执行 Must 启动；心跳在 bootstrap.Heartbeat 内判断 enabled，两个入口各保留一行无返回值调用。未配置/false 时直接返回；已开启时仍按原配置契约校验 metrics.dt。采样错误仅记录，panic 在自身 goroutine 内恢复并记录堆栈，随后结束心跳，不影响主服务。不提供关闭函数或故障通道，不参与 WaitingExit。worker 独立校验共享配置，忽略 http.*。
- 保留已实现的 HTTP Recovery/健康路由、初始化驱动超时、可选独立心跳、日志工厂与存储 DAO 自身失败释放。没有新增依赖版本或业务补偿框架。
- 测试调用真实 main 子进程，验证启动 panic、敏感信息不输出、worker 忽略 HTTP 配置；API 隔离集成覆盖监听失败和正常运行后 SIGTERM 退出 0。WaitingExit 单测及子进程验证按序关闭、异常继续、总期限和二次信号强退。
- 配置文件读取失败改为安全 panic 信息；HTTP Must 包装也不透传下游监听错误详情。未知业务 panic 不在装配层统一捕获。
- 模型移动等原有工作区改动不纳入本项；任务可靠性算法不变，仅增加 MustStartSthTask 失败快速终止的入口包装。

## 状态勾点（关键里程碑）

- [x] 用户指定启动本项，限定本轮仅设计登记。
- [x] 核对当前实现、协议与编号，登记最小方案和验收标准草稿。
- [x] 用户审阅方案，确定进入代码实施。
- [x] 建立隔离验证基线，落实初始化与退出接口。
- [x] 完成 HTTP 装配、Must 启动、心跳与健康路由，按最新边界移除装配层启动回滚。
- [x] 完成自动化验证与全量回归，区分并报告存量失败。
- [x] 用户完成高风险变更人工验收。
- [x] 按协议收尾并归档。

## 验收结论

收尾结论（2026-11-04）：用户明确确认 D02 可以收尾，启动命令、配置、Ready 及退出契约的人工确认完成，设计与过程记录归档至 docs/history。本次仅更新文档状态和引用，不修改功能代码，不重新运行测试；保留各阶段测试结果、未执行项及范围外问题，不将归档视为新增验收通过。后续显式 HTTP/gRPC 通道及相关验证见 [D03](API可选gRPC装配-design.md)。

以下为各阶段记录；旧接口、任务实现、存量失败及“待确认/不归档”等描述表示当时状态，以本节收尾结论及后续扩展记录为准。

main 测试分级（2026-11-04）：保留两个入口的快速失败子进程测试在 -short 下执行；API 完整入口集成增加 testing.Short 跳过，并保留显式隔离 MySQL 参数门槛与 miniredis。未改生产代码。按用户要求仅修改及格式化，未运行测试、构建或静态检查。

退出实现合并（2026-11-04）：合并 WaitingExit/waitingExit，保留 closeResource，信号范围、清理顺序、退出码、日志最后关闭及总超时/二次信号强退逻辑不变。测试改走公开入口；增加跨平台 HTTP 触发的清理/日志卡住子进程用例，保留 Linux 正常信号及二次信号用例，补充 closeResource 错误传播断言。二次信号用例的总退出期限长于父测试期限，避免超时误报。按用户要求仅修改及格式化，未运行测试、构建或静态检查。运行栈就绪检测属于测试实现约束，尚未实测。

HTTP 参数收紧（2026-11-04）：HTTPOptions 改为私有 httpOptions，删除 Address 字段及测试中的无效地址初始化；MustStartHTTP 直接构造监听地址。按用户要求仅修改及格式化，未运行测试、构建或静态检查，历史验收结果不代表本轮已验证。

删除 HTTP 转发层（2026-11-04）：移除 StartHTTP，net.Listen 直接放入 MustStartHTTP；保留 serveHTTP、Shutdown 与既有退出行为。同步端口冲突、请求排空、强制取消和并发关闭测试，未新增同等转发函数。沿用用户仅修改要求，本轮只格式化，未运行测试、构建或静态检查；下文保留 StartHTTP 的记录为此前状态。

HTTP 装配内聚（2026-11-04）：main 步骤4 仅保留 server := bootstrap.MustStartHTTP(cfg)。Gin 模式、Recovery、路由、配置映射及就绪发布移入 MustStartHTTP；Shutdown 内部撤销就绪，入口直接传 server.Shutdown。保留 StartHTTP 底层监听/排空实现及退出行为，worker 不启动 HTTP。补充装配测试覆盖模式、健康路由和关闭后 Ready 为 503；沿用当前仅修改、不运行测试的约束，本轮仅格式化，未运行测试、构建或静态检查。

退出接口简化（2026-11-04）：核对全部 Go 调用方后，确认 backgroundDone 仅测试使用、两个生产入口均传 nil。已移除参数及分支，同步两个入口、单元测试签名与子进程 HTTP 故障用例。按用户要求，仅修改及格式化，未运行测试、构建或静态检查；历史验收结果不代表本轮变更已验证。

最新心跳简化（2026-11-04）：移除 Heartbeat 返回值、关闭/等待及故障通道，两个入口不再向 WaitingExit 接入心跳。采样错误只记录，panic 在自身 goroutine 内恢复并结束心跳。移除旧心跳停止/故障传播契约测试，WaitingExit 通用后台故障用例保留并更正命名。按用户要求，仅修改和格式化，未运行测试、构建或静态检查；下文历史验证不构成本次变更通过的证据。

心跳开关内聚调整：判断移入 Heartbeat，两个入口与既有测试调用签名同步更新。按用户明确要求，本轮仅修改，未运行测试、构建或静态检查。


可选心跳补充验证（2026-11-04）：相关 cmd/bootstrap/global/routes 单测、全量 build/vet 通过；新增配置用例覆盖未设置开关或显式 false 时忽略无效采样间隔，心跳用例覆盖启动错误返回 nil 而非 panic，以及正常停止。初次检查受 Go 缓存权限限制，授权重跑通过。本轮未重跑真实存储集成、race 或覆盖率；原有专用存储集成用例未设置参数时明确跳过。

最新 Must 启动边界验收（2026-11-04，Asia/Shanghai）：

- 两个 main.go 已恢复直线式 Must 调用，无入口 recover/defer 清理或 run/start 回调。启动失败由未捕获 panic 非零退出；WaitingExit 只接管全部启动成功后的运行退出。初始化失败不保证日志刷新，初始化阶段不启用退出信号看门狗，这是用户确认的责任边界。
- Windows 相关单测、真实 main 子进程失败测试、专用 MySQL 8.0.36/miniredis 存储集成与端口冲突测试通过；全量 go build ./...、go vet ./... 通过。初次构建/vet 受缓存访问权限限制，授权重跑通过。
- Linux 隔离环境：相关包 go test -race -count=1 -timeout=60s 通过；真实 API main 启动后 /ready 成功，SIGTERM 清理退出 0；端口冲突直接 panic。WaitingExit 子进程覆盖 SIGINT/SIGTERM、后台失败、关闭卡住、日志关闭卡住、二次信号强退；启动 panic 用例明确不执行运行后关闭步骤。
- 当前覆盖率：所测包联合 85.5%，API 90.0%、worker 0.0%、bootstrap 88.6%、global 89.5%、routes 100%。worker 仅在子进程验证启动失败，panic 子进程未计入正常覆盖率采集，也没有运行真实任务成功路径；不得据此宣称 worker 或全仓达标。原始覆盖率保留在 tmp/d02-must.out。
- 当前代码与本轮修改前快照 tmp/d02-before-must 的隔离全量 go test -count=1 -timeout=45s ./... 均仅 internal/task 因默认 MySQL 初始化失败而失败，其余已执行包通过。本轮未重跑全量 race/staticcheck，较早记录仅作当轮证据。
- 本轮只使用专用 gtank-d02-must-mysql 与内存 Redis；临时 Go 容器自动删除，MySQL 容器和匿名卷在测试后清理。任务算法及已有模型移动未纳入改造，D02 保留人工验收状态。

启动 panic 与资源关闭职责拆分补充验收（2026-11-04，Asia/Shanghai）：两个入口均直接恢复启动 panic，删除 startup/nil 的 Cleanup 调用。新增实际入口测试用配置值的 String 方法在日志创建后触发 panic，无须外部存储；断言退出码 1、诊断及堆栈写入文件、敏感 panic 值不输出、日志最终关闭。新增 Cleanup 不吞调用方 panic 的测试，保留关闭函数 panic/error 隔离、逆序关闭及子进程超时/信号验证。Windows 相关单测、全量 build/vet 通过（缓存权限限制经授权重跑）；隔离 Linux 相关包 race 及真实子进程测试通过。隔离全量测试仍仅 internal/task 的默认 MySQL 初始化失败，与此前已确认的存量问题一致，其余已执行包通过。本轮真实存储集成明确跳过，未重测覆盖率；下文覆盖率为各自当轮记录。Linux 临时容器使用无外部网络模式并自动删除。

顺序装配补充验收（2026-11-04，Asia/Shanghai）：

- Windows：实际入口及 bootstrap/global/routes 测试通过，专用 MySQL 8.0.36 与 miniredis 的 API 集成实际执行；`go build ./...`、`go vet ./...` 通过。初次构建/vet 受 Go 缓存访问权限限制，授权重跑通过。
- Linux：`go test -race -count=1 -timeout=60s -coverprofile=/out/d02-defer.out ./cmd/... ./internal/bootstrap ./internal/global ./routes` 通过。真实子进程覆盖正常 SIGINT/SIGTERM、二次信号、清理卡住、初始化卡住、日志关闭卡住、启动错误及 panic；退出码符合预期。defer 测试覆盖顺序、清理错误/panic 后继续回收；HTTP/心跳关闭测试验证后台故障不会被并发退出掩盖。
- 当前所测包联合覆盖率 85.3%（API 75.4%、worker 43.5%、bootstrap 93.4%、global 98.7%、routes 100%）。原始文件保留在忽略目录 tmp/d02-defer.out。入口分包及全仓核心覆盖率未声明达标，worker 真实任务成功路径仍不执行。
- 隔离全量 `go test -count=1 -timeout=45s ./...`：当前代码与本轮修改前快照均仅 internal/task 因默认 MySQL 初始化失败而失败，其余已执行包通过。快照在 tmp/d02-before-defer，用于保留当时未提交的工作区基线；没有 stash 或覆盖用户改动。当前模型文件移动属于已有外部改动，不属于本次范围。
- 没有修改任务可靠性算法；本轮未重跑全量 race/staticcheck，其此前存量结论见下文。专用 gtank-d02-defer-mysql 容器与匿名卷在验收后清理，临时 Go 容器自动删除；默认开发存储未访问。
- 本轮替换的是退出编排基础代码，按高风险变更保留人工确认步骤；自动化验证已完成，D02 仍不归档。

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
