<!-- D03 | 状态：已完成 -->
# API 可选 gRPC 装配

## 问题

API 当前只装配 HTTP。用户要求在 bootstrap 以 MustGrpcServ 风格增加可选 gRPC 服务，保持入口直接顺序启动。此前任务删除为用户已有修改，不恢复任务实现。

## 规则与约束

- 遵循 [项目规范](../../项目规范.md)、[AGENTS](../../AGENTS.md)及[设计模板](../设计文档模板.md)。
- 保留 [D02 启动边界](服务启动与退出闭环-design.md)：启动失败直接 panic，不统一恢复或跨组件回滚；运行后超时/二次信号强退，日志最后关闭。
- 编号沿用当前会话已确认的 D02 后继 D03；不读取归档正文。

## 方案与决策记录

### 当前调整：版本信息接口

命名收敛：协议目录改为 api/version/v1，Go 包名 versionv1，protobuf 包名 version.v1；处理器沿用工作区已有 controller/rpc 目录并将 Go 包名同步为 rpc；命令行入口改为 cmd/grpc_version。RPC 全名变为 /version.v1.VersionService/Version，不保留旧 demo.v1 别名，客户端须重新生成代码。文档改为 gRPC版本查询.md。仅更名及重新生成，不增加层级或封装，配置键、响应字段和生命周期不变。下文旧路径仅表示各次验收时的历史状态。

用户要求以版本查询替换 SayHello。当前协议为 `version.v1.VersionService/Version`，空请求、响应字段 name/desc/version；服务端直接读取 global.AppConf 的 name/desc/ver，与 HTTP 根路径一致，不增加配置快照或装配层。删除 Greeter 协议及生成文件，重新生成 version.proto 对应文件；这是示例接口替换，不保留旧 RPC。客户端删除 name 参数，以 JSON 输出三个字段（包括空字符串）。测试验证配置映射、缺省空值、取消，以及 HTTP 与 gRPC 响应一致。当前约定优先于下文此前 Greeter 设计与验收记录。

- main 一行 `rpc := bootstrap.MustGrpcServ(cfg)`；开关在函数内部判断，禁用返回 nil，nil 接收者的 Done/Shutdown 安全。worker 不读取 gRPC 配置、不启动 gRPC。
- 使用独立 TCP 监听，不与 Gin 共用端口。新增 grpc.enable（默认 false）、address（默认 127.0.0.1:9090）、drain_timeout（默认 5s）、connection_timeout（默认 5s）。仅启用时校验；HTTP 与 gRPC 的顺序排空期限之和须小于总退出期限，为存储及日志保留时间。
- 默认明文且仅回环地址；跨主机部署需明确配置地址并通过可信代理/网络提供传输保护。本项不添加 TLS、鉴权或反射；当前提供可删除的版本查询接口，不实现生产业务。
- MustGrpcServ 内直接注册标准 gRPC Health 服务，业务服务在同处追加生成代码的 Register 调用；不引入额外转发函数、注册回调、Lifecycle 对象。
- 使用 gRPC 官方 Go 库；统一 unary/stream panic 恢复为安全 Internal 状态，日志携带方法及堆栈，不输出请求、metadata 或 panic 值。
- 固定 google.golang.org/grpc v1.85.0；其模块要求同步升级 x/net、x/crypto、x/text、protobuf，并增加 genproto/googleapis/rpc。未引入中间件框架。退出实现参考 [gRPC 官方优雅关闭说明](https://grpc.io/docs/guides/server-graceful-stop/)。
- WaitingExit 显式接收 httpDone、grpcDone 两个结果通道，以普通 select 等待信号或任一服务停止；删除 Cleanup.Done 和 reflect.Select，不为未知数量的服务增加动态编排。Cleanup 只保留 Name/Stop。API 传 server.Done()、rpc.Done()；worker 传 nil、nil，禁用 gRPC 时 rpc.Done() 返回 nil，nil 分支自然不参与等待。收到错误、nil 结果或已关闭通道均触发失败退出；nil 结果/关闭通道的诊断区分 HTTP 与 gRPC。故障与信号并发时各 Shutdown 仍检查 Serve 结果，关闭顺序、异常隔离、日志最后关闭及总期限/二次信号强退不变。
- API 关闭顺序 HTTP → gRPC → 存储 → 日志；gRPC GracefulStop 排空，阶段期限到达 Stop 强制断连并返回超时错误，总期限看门狗仍保留。handler 必须遵循 context 取消；不保证终止忽略取消的业务 goroutine。
- Ready 保持 D02 的 HTTP 生命周期语义，不扩展为跨协议/下游健康探测。gRPC Health 表示本监听服务状态，不承诺业务或存储健康。
- 同步已有 worker 快速入口测试：任务删除后不再期待 MySQL 初始化失败，改用日志配置失败验证忽略 HTTP/gRPC 配置。
- 子进程信号测试检测 WaitingExit 所在的 select 等待栈，不固定模块名 serv_tpl，不增加生产测试钩子。正常信号场景传两个 nil 通道，覆盖 worker 的等待方式；故障场景传显式 HTTP 通道。单元测试覆盖两个协议的错误、关闭通道和 nil 结果，以及 nil 通道跳过。

### 此前方案：最小业务调用示例（已由版本接口替换）

- 用户要求为现有 gRPC 服务增加 demo，作为本项接入示例继续实施，不新建装配抽象。
- `api/demo/v1/greeter.proto` 定义 Greeter.SayHello，生成 Go 消息与服务代码同目录提交；协议目录仅承载契约，不包含业务实现。
- `controller/grpcdemo/greeter.go` 实现问候接口：去除名称首尾空白，名称须为 1–64 个 Unicode 字符，否则返回 InvalidArgument；遵循请求 context 的取消/超时，不访问存储。
- MustGrpcServ 在监听前直接注册示例服务，main 无额外代码，仍受 grpc.enable 控制。示例不代表生产业务功能，复制模板后可删除对应注册及文件。
- `cmd/grpc_demo/main.go` 为命令行客户端，提供 addr/name/timeout 参数，默认回环明文地址和 3 秒期限；连接始终关闭，失败输出诊断并非零退出，不增加业务日志框架。
- 生成代码使用官方 protoc 与 Go 插件，工具放 tmp，不修改系统 PATH、不加入运行时依赖。记录固定版本与再生成命令。正常构建和使用示例无需安装 protoc。
- 单元测试覆盖空白/长度/中文/取消边界；通过实际 MustGrpcServ 随机回环端口验证 protobuf RPC 往返和状态码，无真实 MySQL/Redis 依赖。
- 启动、调用及固定版本再生成命令统一维护在 [gRPC 版本查询](../gRPC版本查询.md)，本设计不复制操作说明。protobuf 由生成代码直接引用，go.mod 改为直接依赖，版本不变；没有新增运行时依赖。

## 状态勾点（关键里程碑）

- [x] 用户启动需求，登记最小装配方案。
- [x] 配置、装配、故障传播及退出接线实现。
- [x] 配置边界、禁用、监听失败、Health RPC、panic、排空/超时及故障测试。
- [x] 构建、vet、隔离全量与竞态验证，记录覆盖率及 staticcheck 限制。
- [x] 按用户要求改为两个显式通道加普通 select，同步调用方与测试。
- [x] 显式通道调整后的验证（在后续 demo 验证中完成）。
- [x] 人工确认后按协议收尾归档。
- [x] Greeter 协议、实现、客户端与使用说明。
- [x] demo 单元测试及实际 RPC 验证。
- [x] SayHello 替换为 Version，协议、生成文件、实现、客户端及使用文档同步。
- [x] 版本查询验证与验收登记。
- [x] 版本查询命名收敛及对应验证。

## 验收结论

收尾结论（2026-11-04）：用户明确确认 D03 可以收尾。可选 gRPC 装配、显式通道退出、版本查询及职责命名收敛按本项范围交付，设计与过程记录归档至 docs/history。本次仅更新文档状态及引用，未修改功能代码、未重新运行测试；各阶段验证结果和未执行项保留如下，不将收尾视为新增测试通过。全仓覆盖率不足、staticcheck 未执行及 TLS/鉴权/反射等范围外能力仍按原边界保留。D02 不随本项归档。

以下为各阶段记录，其中旧路径、待人工确认等描述仅表示当时状态，以本节收尾结论为准。

命名收敛验收（2026-11-04）：协议及官方重新生成文件位于 api/version/v1，Go 包 versionv1、protobuf 包 version.v1；处理器沿用用户已移动的 controller/rpc，包名与引用同步为 rpc；客户端入口为 cmd/grpc_version。使用文档改为 [gRPC 版本查询](../gRPC版本查询.md)，更新命令、代码链接和再生成路径。可执行代码已无旧 demo 包名或引用；下文历史验收保留当时路径。`go test -count=1 -timeout=45s ./controller/rpc ./internal/bootstrap ./cmd/grpc_version`、全量 `go build ./...`、`go vet ./...` 均通过（首次缓存权限受限，授权后重跑）；包含新 RPC 全名的真实请求及 HTTP 字段一致性验证。未新增封装层、依赖或配置，不改响应和退出语义；未重跑全量测试、race 或真实存储集成。外部旧客户端需要重新生成并更新 RPC 全名。

版本查询替换验收（2026-11-04）：已移除 Greeter/SayHello 及旧生成文件，官方工具重新生成 Version 协议；客户端去掉 name 参数，以 protojson 输出 name/desc/version（包括空值）。`go test -count=1 -timeout=45s ./controller/grpcdemo ./internal/bootstrap ./cmd/grpc_demo` 通过；Version 单测覆盖缺省空值、name/desc/ver 键位、取消和超时，处理器覆盖率 100%。真实回环 RPC 与 HTTP 根路径的字段一致性测试通过。全量 `go build ./...`、`go vet ./...` 通过（首次缓存权限失败，授权后重跑）。无新增依赖，未改变入口或退出流程；本轮未重跑全量测试、race、真实存储集成或命令行客户端连接真实 API。旧示例客户端不兼容新 RPC，调用方式见使用文档。D03 继续等待人工确认。

Greeter 示例验收（2026-11-04）：

- 已提交 `.proto`、官方生成的两个 Go 文件、服务端、命令行客户端及 [使用说明](../gRPC版本查询.md)。既有 API main 不变，仅在 MustGrpcServ 中追加注册。
- Windows `go test -count=1 -timeout=45s ./...`、`go build ./...`、`go vet ./...` 均通过。首次测试受 Go 缓存权限限制，授权后重跑通过。真实存储集成未配置参数，按约定跳过。
- `go test -count=1 -cover ./controller/grpcdemo` 覆盖率 100%；含空白、64/65 字符、中文、无效 UTF-8、nil 请求、取消和已过期 context。该数字不代表全仓覆盖率。
- TestGRPCDemoRoundTrip 经实际 MustGrpcServ 随机回环监听验证：中文请求及响应序列化、非法参数状态码、失败后再次成功调用；不初始化真实存储。
- 无网络、只读源码 Linux 临时容器内 `go test -race -count=1 -timeout=60s ./controller/grpcdemo ./internal/bootstrap` 通过；同时验证了当前普通 select 的信号退出、超时及二次信号强退。容器自动删除。
- 本轮没有重跑真实 API + MySQL/Redis 集成、全仓 race 或全仓覆盖率；命令行客户端已构建，实际 RPC 验证通过生成客户端执行，未单独运行命令行客户端连接真实 API。staticcheck 仍未执行。
- 生成工具保留在忽略目录 tmp/grpc-tools，未改变系统 PATH；无新增运行时依赖。D03 仍待人工确认，不归档。

此前显式通道调整（2026-11-04）：已删除 Cleanup.Done、reflect.Select，更新两个入口、单测与子进程测试。当轮按用户要求仅修改及格式化，未执行测试、构建或静态检查；后续验证见上述 Greeter 示例验收。

此前 gRPC 接入验收（2026-11-04）：实现及自动化验证通过，基础退出接口变更保留人工确认。

| 检查 | 结果 |
| --- | --- |
| Windows `go build ./...`、`go vet ./...` | 通过；首次受 Go 缓存权限限制，授权后重跑通过 |
| Windows `go test -count=1 -timeout=45s -coverprofile=tmp/d03-coverage.out ./...` | 通过；真实存储集成未设置参数，按约定跳过 |
| `go tool cover -func=tmp/d03-coverage.out` | 全仓 53.0%，bootstrap 82.4%、global 91.1%；新增 grpc.go 为 56/63 语句（88.9%），两个恢复拦截器均 100%。全仓未达到规范 85%，不宣称核心整体达标；main 子进程未计入父进程覆盖率 |
| Linux `go test -race -count=1 -timeout=60s ./...` | 全量通过；源码/模块缓存只读，无网络容器，不连接真实存储；包含 SIGINT/SIGTERM、总期限、二次信号及日志卡住强退 |
| Linux 真实 API `go test -race -count=1 -timeout=60s -run '^TestEntry' -v ./cmd/api` | 显式隔离 MySQL 8.0.36 + miniredis；HTTP 单协议、HTTP+gRPC Health 成功及 SIGTERM 退出 0；HTTP/gRPC 端口冲突均 panic 非零退出；新增非法 gRPC 配置快速失败通过 |
| staticcheck | 本机命令未安装，未运行；不以 vet 替代该项 |

gRPC 组件测试覆盖禁用无监听、监听冲突、实际 Health 请求、重复关闭、流式 RPC 正常排空与超时强停、Serve 故障保留、两种 handler panic 恢复。WaitingExit 覆盖 HTTP/gRPC 任一通道故障、关闭通道、nil 通道跳过及后续清理。

验证期间首个覆盖率命令未引用 PowerShell 参数导致 `.out` 被当成包名，修正参数引用后全量通过，属于命令调用问题；没有将此失败归为代码缺陷。旧任务测试已由用户删除，不再造成默认测试失败；本次同步 worker 旧测试断言，未恢复任务实现或修改任务框架算法。

本次 gtank-d03-mysql 容器及匿名卷已删除，Go 容器 --rm 自动删除；覆盖率留在忽略目录 tmp/d03-coverage.out。未访问默认开发存储。全量竞态后只增加 API 集成/配置失败测试及注释，新增入口用例已在隔离环境带 race 实际运行。

遗留边界：版本查询不代表生产业务 RPC，未实现 TLS/鉴权/反射；Health 不表示存储健康，HTTP Ready 不表示 gRPC 已就绪；启动失败不回滚先前组件；强停无法终止忽略 context 的业务 goroutine；全仓覆盖率不足及未执行 staticcheck 保留。后续接入业务服务时在 MustGrpcServ 的监听前注册，并补充业务 deadline/取消测试。
