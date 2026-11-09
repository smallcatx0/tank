<!-- D32 | 状态：待验收 -->
# Prometheus与链路追踪接入

## 问题

用户选择 Prometheus 最小接入方案，并要求追加链路追踪。用户已确认完善后的设计并启动实现；本轮不提交 Git，不部署观测平台。

当前已有 Prometheus 客户端依赖与 [Kafka Collector](../../pkg/kafka/metrics.go)，但 API、worker 没有抓取端口；HTTP 仅有请求关联日志，gRPC 未统一传播链路；[Trace 中间件](../../internal/httpmd/trace.go)只处理 `x-b3-traceid`，不是完整分布式追踪。[系统心跳](../../internal/bootstrap/heartbeat.go)输出日志，与 Prometheus 指标不同。

## 规则与约束

- [项目规范](../../项目规范.md)
- [文档与进度管理](../../AGENTS.md)
- [响应契约](../../internal/resp/README.md)
- [日志约定](../../pkg/logger/README.md)
- [Kafka 消费契约](../../pkg/kafka/README.md)

## 方案与决策记录

### 1. 范围与默认方案

| 信号 | 首版方案 | 不纳入首版 |
| --- | --- | --- |
| 指标 | 独立监控端口、独立 Registry、Go/进程 Collector、HTTP/gRPC 请求指标、现有 Kafka Collector、抓取配置 | OTel Metrics 管线、业务指标全集、Kafka lag、数据库 exporter、仪表盘与告警平台部署 |
| 链路 | OpenTelemetry Go SDK、OTLP/HTTP、HTTP/gRPC 服务端与出站调用、Kafka 回调与生产端传播、日志关联 | 自动 SQL/Redis span、尾部采样、全量错误链路保留、完整生产者组件 |
| 日志 | 沿用 zap，增加 span_id 关联能力 | OTel Logs、日志收集平台部署 |

Prometheus 和 tracing 独立开关，模板默认关闭，关闭时不监听监控端口、不连接导出后端。不会要求业务处理器创建 Registry、Exporter 或管理关闭过程。

默认链路路径为应用 → OTLP Collector → 兼容追踪后端。部署侧可提供已有 Collector；本方案不绑定 Jaeger、Tempo 等具体存储。指标由 Prometheus 直接抓取应用，不绕经 Collector。

首版只扩展现有装配和关闭流程，不重构通用生命周期框架、不新增通用遥测插件或可配置脱敏规则引擎。现有 HTTP→gRPC 顺序排空与 readiness 语义继续保留，不将进程级就绪重构混入本需求。

### 2. 配置契约

```yaml
observability:
  service_name: order-api
  metrics:
    enable: false
    address: 127.0.0.1:9091
    shutdown_timeout: 2s
  tracing:
    enable: false
    endpoint: http://127.0.0.1:4318/v1/traces
    sample_ratio: 0.1
    export_timeout: 3s
    shutdown_timeout: 5s
```

- `service_name`：仅 tracing 启用时必填，API/worker 用不同名称，如 order-api、order-worker；实例 ID 每次进程启动生成一次。版本沿用现有 `ver`（worker 未配置可省略），环境沿用 `env`，写入 Trace Resource。只开指标时服务标识来自抓取标签，不强制重复填写。
- 指标路径固定 `/metrics`，避免额外配置面。HTTP 监控服务器固定设置读头 5s、读/写 10s、空闲 30s；最大并发抓取 5、Handler 超时 5s。超时不能终止任意自定义 Collector，首版 Collector 禁止远程查询或阻塞操作。
- 指标地址需为合法 host:port；同机 API 与 worker 分别配置 9091/9092，容器内按部署需要改为 0.0.0.0。只向可信监控网络开放。
- `endpoint` 是完整 OTLP/HTTP traces URL，显式传给 exporter；首版不增加 OTLP/gRPC 选项。HTTPS 使用系统信任根，私有 CA 由部署加入信任；跨信任边界必须 HTTPS。首版连接可信网络内无应用层认证的 Collector，不新增认证配置入口；需要认证/mTLS 的部署属于明确未覆盖场景，实施前另行确定，不能声称现方案支持。
- YAML 控制开关、Resource、端点、采样与期限；这些值必须显式传给 SDK，不能仅设置端点就宣称屏蔽所有 OTEL_* 回退。固定空认证 headers、无压缩、明确重试与 SpanLimits/批处理参数；TLS 使用显式标准 HTTP Transport。实施测试注入冲突 OTEL_* 值，验证端点、凭据、TLS 与资源属性不被静默改变；禁止为此修改进程环境。
- exporter 使用专用、未埋点的 HTTP Client，显式设置网络/总请求期限和 TLS，不能复用 otelhttp 业务客户端，避免遥测导出递归产生 span。SDK 的 WithHTTPClient 会影响超时/TLS 选项优先级，实施时以实际导出测试为准。
- `sample_ratio` 必须在 [0,1]；使用 ParentBased(TraceIDRatioBased)，比例控制本服务新建根链路，已存在父链路遵循父采样决定。联调设为 1，生产按流量调整；错误链路不保证全量保留。
- 导出采用 BatchSpanProcessor，首版固定最大队列 2048、最大批量 512、批处理等待 1s，并设置导出超时。队列满时允许丢失 span，不让业务等待遥测后端。
- 现有 `metrics.enable/dt` 是系统日志心跳，原语义保留，说明文档必须明确区别，不复用为 Prometheus 开关。
- 仅校验已启用功能的配置；关闭时忽略对应端点/期限，不创建监听器或 exporter。时长必须为正且无溢出；采样比例拒绝 NaN/Inf；URL 必须为 http/https、含 host、不含 userinfo/query/fragment；指标端口为 1..65535。总退出预算在全部配置解析后统一校验，不各自局部判断。
- 观测配置错误按第 8 节降级；Collector 不可达不阻塞业务启动或就绪。每批导出的总期限含 SDK 重试，重试次数受时间预算限制，不另建重试 goroutine。批处理有界、非阻塞入队。导出失败/关闭失败经包装层限频写入 zap（不回显 URL 或凭据），不新增全局 OTel ErrorHandler；SDK 自身诊断不保证全部被应用捕获。

### 3. Prometheus 指标与注册

每个进程在装配阶段创建一个 `prometheus.NewRegistry()`，显式注册 Go Collector、Process Collector 与应用指标；通过 `promhttp.HandlerFor` 暴露。关闭状态不创建无用 Registry。禁止业务包通过 init 或默认全局注册器自动注册。

| 拟定指标 | 类型 | 标签/语义 |
| --- | --- | --- |
| service_http_requests_total | Counter | method、route、status、outcome；完成请求数，outcome 为有限结果分类 |
| service_http_request_duration_seconds | Histogram | method、route；完整处理耗时 |
| service_http_inflight | Gauge | 无标签；当前请求数 |
| service_grpc_requests_total | Counter | service、method、code；RPC 结束次数 |
| service_grpc_request_duration_seconds | Histogram | service、method；从开始到结束，流式 RPC 是整条流时长 |
| service_grpc_inflight | Gauge | 无标签；当前 RPC 数 |
| kafka_consumer_* | 已有类型 | 保留现有名称、标签与语义，不重新实现 |

- HTTP 路由取 Gin 路由模板；未匹配路径归并 `unmatched`，非常用方法归并 `OTHER`。gRPC 首版只观测已注册业务方法，不以任意输入方法名形成标签。
- 初始请求耗时桶为 0.005、0.01、0.025、0.05、0.1、0.25、0.5、1、2.5、5、10 秒及 +Inf；长流耗时本轮只做基本分布，不承诺精准分位数，后续按需求另设桶。
- 请求和错误率由请求 Counter 的 status/outcome 或 code 标签计算，避免重复维护易失配的错误 Counter。Kafka 继续使用已有事件计数。
- 不将 trace_id、用户 ID、消息 key、offset、URL 查询参数、错误原文作为指标标签。service/env/job/instance 在 Prometheus 抓取配置标注；不为每项 Go 指标重复增加业务标签。
- `/metrics`、`/healthz`、`/ready` 与 gRPC health 方法不计入业务指标和业务追踪，减少自观测噪声。
- HTTP 中间件顺序为 tracing/关联信息 → metrics/访问日志 → recovery → handler，在 recovery 完成后记录实际状态与 outcome 并结束 span，响应已写出时不假设一定能改成 500。
- gRPC 指标使用 stats.Handler 的 Begin/End 事件统计已注册方法，每个 RPC 结束一次，从 End.Error 取状态，覆盖进入处理流程后的解码失败。当前 grpc v1.85.0 的未注册方法直接返回 Unimplemented，不保证产生 Begin/End；因此不能声称 StatsHandler 能完整统计未知方法。首版将未知方法与握手/连接失败排除，不为监控另装 UnknownServiceHandler 改变分发流程。已注册方法在 routes.RegisterGRPC 后从 GetServiceInfo 构建只读集合，并在 TagRPC 之前过滤未知方法，避免创建无 End 的 span；已注册但未实现的方法仍按 Unimplemented 计数。
- tracing 与 metrics 共用一个显式装配的 StatsHandler 组合：TagRPC 先执行追踪，再附加计时上下文；HandleRPC 将同一 context 交给两者，连接事件同样转发，不新建 span。只开指标时不创建追踪 handler；health/未知方法使用相同过滤规则；关联信息与日志的公共 stats 处理不依赖 metrics 开关，tracing 关闭时仅提取透传上下文。应用仍保留现有 unary/stream recovery；完成日志在 stats End 中输出且仅输出一次。
- Kafka 在 `NewConsumer` 后、Run 前向传入注册器注册；注册失败时警告并继续消费，不覆盖现有注册项。通过装配参数传递 Registry，不通过 global.KafkaCli 反向寻找。测试/重复装配不得污染其他实例。
- `kafka_consumer_pending` 只代表本轮等待处理的消息，不是消费组积压。

### 4. 链路传播与现有请求标识

标准传播使用 W3C Trace Context（traceparent/tracestate）；首版不自动转发 baggage，也不宣称支持完整 B3 协议。

| 情况 | 行为 |
| --- | --- |
| tracing 开启，合法 traceparent | 提取远端父上下文，创建 server/consumer span |
| tracing 开启，无父上下文或非法 traceparent | 创建新根 span，不沿用非法输入 |
| tracing 开启，仅传入旧 x-b3-traceid | 不能当作完整父 span；创建新链路，不新增 legacy_request_id 兼容字段 |
| tracing 关闭 | 不创建/导出 span；保留合法 W3C 上下文的透传，HTTP 无合法 W3C 时保留现有 B3 请求关联行为 |

开启追踪后，日志 `trace_id`、响应体 `request_id`、响应头 `x-b3-traceid` 统一使用当前 span 的 32 位 TraceID；`span_id` 为当前 span ID。旧响应字段名保留，但不承诺继续原样回显只有 `x-b3-traceid` 的输入：这是明确的契约调整，实施时同步响应文档及测试。不通过伪造父 span 来保留旧 ID。

`requestctx.TraceID(ctx)` 优先读取有效 OTel SpanContext，再回退现有请求关联数据；新增 SpanID 读取能力，但只返回本地 span 的 ID，远端上下文（IsRemote=true）不作为本服务 span_id。耗时起点仍单独记录，不因创建子 span 而重置。未采样 span 仍可携带有效 ID，因此日志有 trace_id 不保证后端能查询到该链路。

关闭 tracing 时走显式的 extract/inject 透传分支，不依靠 no-op Tracer.Start 创建本地 span。合法 W3C 优先于旧 B3；HTTP 的 Gin key、响应头和 response body 必须同步这一优先级，不能只修改 TraceID(ctx) 导致日志与响应不一致。无 W3C 时仅 HTTP 保留已有请求关联 ID，gRPC/Kafka 不为了日志伪造 span 或父上下文。未启用或无合法上下文时的 span_id 为空；业务自行创建的有效本地 span 除外。

每个请求/回调 context 是唯一传播入口，禁止另用全局变量保存当前 span。SDK 生成代码保持纯协议，不引入遥测依赖。HTTP/gRPC 出站必须使用传入 context 并保留显式超时。

### 5. 各入口与出站接入

**HTTP**：Gin middleware 在业务前开始一个 SERVER span，业务结束后按路由模板命名；记录方法、路由模板、响应状态，不记录正文、查询参数和任意请求头。HTTP 5xx/panic 标记 Error，4xx 作为状态属性。当前 Recovery 在响应已经写出后不会改成 500，Response 在请求取消时也可能不写响应，因此不得仅凭 Writer.Status 推断成功。Recovery 需附加内部 panic 标记（不存 panic 值）；请求结束同时读取该标记、context 和实际状态。outcome 取 panic、canceled、deadline_exceeded、server_error、client_error、ok，按此前后顺序判定；status 保留实际 Writer 状态，不伪造 499/500。出站使用官方 otelhttp Transport，显式注入 Provider/Propagator；仍由业务关闭响应体和控制超时。

**gRPC**：使用官方 otelgrpc 的 NewServerHandler/NewClientHandler（StatsHandler），同时保留 unary/stream recovery。客户端通过统一装配入口使用 ClientHandler，不把配置放进生成 SDK。流的 span 生命周期覆盖到 RPC 结束，首版不对每条流消息额外建 span。只记录已注册方法和状态码，不记录 protobuf 正文。HTTP/gRPC 官方 instrumentation 都显式设置 no-op MeterProvider，指标只经本方案 Prometheus 管线，避免双重采集。gRPC 非 OK 的 span 状态遵循锁定版本的官方语义，不把所有业务错误强行改为系统错误。

**Kafka**：首版在 worker 装配处包装业务 Handler，每次回调尝试从消息 Headers 提取 W3C 上下文，创建一个 CONSUMER span，使用消息传播上下文而非整个 Run 的进程级 context 作为父链路；取消和期限仍来自本次回调 context。正常生产消费采用父子关系，不新增长驻 worker 根 span。

- 重试中的每次回调为新的同级 consumer span，共用消息生产端父 span；重投递再次产生新 span，不复用旧 SpanID。topic/partition/offset 可作为受控 span 属性，不作为指标标签。
- 缺少有效父上下文时每次尝试可形成独立根链路，用消息位置辅助关联。首版不为跨尝试关联在组件中新增持久状态。
- wrapper 捕获 panic 只为标记并结束 span，随后原样重新 panic，由现有消费者转换为 Permanent，避免改变重试行为。返回错误仅记录安全分类，不直接把任意错误文本写入 span。
- wrapper 必须同步执行原 Handler，保持 nil/错误返回、期限及取消语义，不启动额外处理 goroutine。结束 span 时同时检查 Handler 返回错误和回调 ctx.Err()：现有 handle 会把超时后返回 nil 视为失败并重试，span 也必须标记超时；wrapper 不改写原返回值，不提前 cancel 原 context。
- 提取消息父上下文前清除回调 context 上继承的 SpanContext，保留取消、deadline 和业务 Value；再提取消息头。无消息父上下文时创建根 span，不能意外挂到 Run 的进程上下文。耗时起点在每次回调入口设置，不能复用 Run 的起点。
- 回调 span 只覆盖业务尝试，不覆盖后续位点提交；提交结果仍由现有指标和日志观测，不能将回调成功误称为消息提交成功。
- 提供 W3C Header 注入辅助能力与一个生产调用示例：创建 PRODUCER span，发送前注入，Broker 确认/失败后结束。验证入口可扩展现有测试生产者；不创建完整业务生产者框架。
- 注入时只覆盖本方 traceparent/tracestate，保留其他 header；traceparent/tracestate 按规范解析；相关头合计最多 4KiB，冲突重名、超限或非法值只丢弃传播上下文并新建根 span，不使业务消息失败或增加重试。相同重名值可折叠。不得修改只读 Message 视图中的原始数据。
- `pkg/kafka` 不 import `internal`。消费 wrapper 与注入适配放在本项目内部遥测包，直接使用已有 kafka.Handler/Message。

### 6. 日志、采样与数据边界

继续用 `logger.L()` 记录日志，新增的 context 字段辅助只生成 zap 字段，不另建日志实例。HTTP/gRPC 请求完成日志、panic 日志和 Kafka 业务回调日志应能关联 trace_id/span_id；系统级日志允许无 span。

只记录固定属性清单。Span 不含密码、token、完整 SQL、Redis 命令参数、请求正文、消息正文或任意 error 原文。首版不增加 SQL/Redis 自动埋点，业务访问存储仍可从现有日志关联 trace_id。

官方 instrumentation 可能自动附带完整 URL、状态描述或异常事件，不能仅约束手工埋点。首选官方配置禁用消息事件并在本项目埋点处控制属性；仅对锁定版本实际产生的敏感字段做固定出口处理：移除完整 URL/原始路径、异常文本、异常堆栈和自由文本 status description，保留状态码、时间、TraceID/SpanID/Parent 及安全属性。采用不修改原数据的轻量导出视图包装，不复制完整 span、不实现规则 DSL、可配置白名单或通用脱敏框架。属性/事件/Link 设置 SDK 数量与长度上限，安全字段允许 HTTP 路由模板、RPC 注册方法、Kafka 消息位置；跨升级通过真实 OTLP payload 测试检查。业务和第三方错误的 zap 日志仍遵守项目规范，不因 span 限制改变日志错误原文策略。

不引入 Prometheus exemplar；trace ID 不成为 metric label。对外不可信父采样标志可能放大采样量，部署应在可信入口规范传播；需要严格采样上限时另行设计，ParentBased 不提供全局流量硬上限。

### 7. 包职责与拟修改范围

| 位置 | 职责 |
| --- | --- |
| internal/telemetry（拟新增） | Registry/Provider 构造、固定指标、StatsHandler 组合、传播、Kafka wrapper、出站辅助；不读配置，不启动业务监听，不 import global/bootstrap/httpmd |
| internal/global | 解析及校验 ObservabilityConfig，只保存配置，不新增全局当前 span |
| internal/bootstrap | 显式注入 Registry/Provider；独立监控服务、错误监听与有界关闭 |
| internal/httpmd、internal/requestctx、internal/resp | HTTP 指标、trace 生命周期、日志与响应关联；resp 只核对现有字段同步，不改变响应结构 |
| internal/task、models/dao/logger.go | 示例回调移除写死的空 trace_id；现有 SQL 日志按需补 span_id，不新增 SQL span |
| cmd/api、cmd/sth_worker | 显式装配业务，仅观测全流程调用 bootstrap |
| routes/grpc、客户端装配点 | 保持业务注册职责；拦截器/StatsHandler 在 bootstrap 装配 |
| test/grpc_trace、test/kafka_producer | 提供可验证的调用链示例，不把 SDK 变为基础设施库 |
| conf、docs、README、项目规范与业务接入指南 | 配置模板、部署抓取示例、最短业务用法；实施时登记 OTel 选型、现有字段的新语义和包依赖方向 |

依赖保持单向：bootstrap → global（解析）及 telemetry（构造）；global 可使用 telemetry 的纯 Options 类型，telemetry 不反向读取全局配置；httpmd → telemetry/requestctx；requestctx 只依赖 context 与 OTel trace API，不依赖 telemetry，避免形成环。pkg/logger 与 pkg/kafka 不 import internal；基础库的提交/后台日志不承诺拥有消费回调 span_id。

TracerProvider、Propagator 显式传入 middleware、StatsHandler 和客户端工厂，不调用 SetTracerProvider/SetTextMapPropagator 修改全局实例。业务确需手工 span 时通过依赖注入取得 Tracer。禁止依赖默认全局 provider 的隐式行为。客户端工厂只装配现有 http.Client/grpc options，不包装成新业务 SDK，不改变地址、连接复用或超时所有权。

HTTP/gRPC 与 Kafka 指标、tracing 的实现可分阶段落地，但共同遵守本设计。依赖版本锁定 OTel 1.45.0、otelhttp/otelgrpc 0.70.0，保留既有 gRPC 1.85.0 与 protobuf 1.36.12。SDK WithResource 会合并环境属性，因此出口视图固定返回配置生成的 Resource。独立 SDK 验证仍使用纯协议 test/grpc_version；追踪示例另放 test/grpc_trace，避免该验证入口依赖根模块。

### 8. 启动、故障与关闭

装配接口统一显式接收单个非 nil Runtime，关闭观测也传关闭态实例，删除可变参数与缺省观测分支。gRPC 客户端由单一 `clientStats` 完成旧字段清理、当前上下文注入和可选官方 handler 转发，顺序保持不变。监控启动改为包内 `startMetrics`，保留 `listenMetrics` 的监听故障测试注入点。其余配置、资源生命周期和导出过滤封装保留。

心跳的配置读取、校验和协程启动统一在 `startHeartbeat` 内完成；删除仅有一个调用点的导出函数 `Heartbeat`，避免重复校验和无复用价值的间接调用。保留协程内 panic 恢复及进程级退出语义。

用户确认观测为非必需能力，故障降级；此决策替代最初观测启动/运行失败终止业务的策略。

- main 保留必需业务配置、日志/存储初始化、HTTP/gRPC/Kafka 启动与关闭顺序。只将可观测性的配置解析、预算校验、创建、故障观察和关闭收敛到 `bootstrap.StartObservability` 返回的对象；main 显式注入 Runtime 并登记一次 Shutdown。不引入 RunAPI/RunWorker。
- 心跳配置从必需的生命周期校验中分离。`StartObservability` 通过 bootstrap 内部函数直接读取 Viper，不保留 `HeartbeatConfig` 或 `LoadHeartbeatConfig`。默认关闭、默认间隔 10 秒，采用 Viper 宽松类型转换（数值小数可能截断）；无法转换的开关按关闭处理，启用后间隔无法转换、非正数或乘以秒会溢出时警告并跳过，不影响其他观测能力。复杂的 metrics/tracing 配置仍保留类型化 Options 与校验。
- 观测配置非法或退出预算不足：警告并整体关闭 metrics/tracing，返回关闭态 Runtime，保留 W3C 透传。关闭态构造不分配后台资源。
- telemetry 初始化返回错误：警告并降级为关闭态；构造函数继续负责自身失败清理。不使用广泛 recover 隐藏程序错误或业务启动 panic。
- 监控监听使用固定 3s 期限（包含主机名解析）。在绑定前核对 API 已启用的 HTTP/gRPC 业务端口，同端口时跳过指标，优先保留业务监听；不同绑定地址但相同端口也保守让位。worker 不受无关 HTTP/gRPC 配置影响。
- 监控端口绑定失败：警告，关闭指标能力，保留已创建的追踪。监控 Serve 运行故障由独立且可结束的观察协程记录，不传给业务退出等待；不自动重启或在运行中修改共享 Runtime。
- Kafka Collector 注册失败只警告并跳过该实例的注册，消费者仍运行，不改变已有注册项；业务消费者的构造/运行失败仍按原契约处理。
- 关闭按 HTTP → gRPC / Kafka → 存储 → 监控 → 追踪 → 日志顺序进行。观测关闭错误警告并继续，不单独改变退出码；每阶段仍有期限，总退出 watchdog 继续兜底。

总退出预算校验仍为业务排空预算 + 已启用 metrics shutdown + 已启用 tracing shutdown + 5s 清理余量 < 总期限，使用防溢出扣减。API 全开建议 40s，worker 55s；不满足时跳过观测，不缩减业务排空期限。业务自身非法配置、存储/HTTP/gRPC/Kafka 启动失败仍快速失败。

监控 Shutdown 超时仍强制 Close；正常关闭等待 Serve 结果观察协程退出后才关闭追踪和日志。观测关闭从进程总期限中预留最后 1s 给日志；剩余不足时取消观测等待，不耗尽总退出预算。Provider 只调用一次 Shutdown，不重复 ForceFlush。SDK 在 context 已取消时可能未通知批处理器就返回，因此保留批处理器句柄，在 Provider 关闭报错时幂等通知其停止，不延长关闭预算。心跳继续采用现有进程级行为，不在本次扩展为通用任务管理框架；业务启动 panic 后不承诺 flush。

### 9. 部署侧准备与拟交付示例

- Prometheus 到每个实例监控端口可达；每个实例独立抓取，不经随机分流的业务负载均衡地址。
- Collector 的 OTLP/HTTP receiver、追踪存储和查询界面由部署侧提供；Collector 本身不是持久追踪存储。
- 应用镜像需验证 CA 信任链；HTTPS 的证书与私有 CA 不提交到仓库。
- 抓取配置和 Collector 最小 pipeline 示例后续平铺为 `conf/prometheus.yaml.example` 与 `conf/otel-collector.yaml.example`；现有 conf/.gitignore 忽略子目录，不能直接放入 conf/observability/ 后强制添加；仅用示例 DNS/端口，不写本机地址和实际凭据。若需要完整本地容器环境，作为独立实施任务，不在本轮部署。

Prometheus 抓取示意（目标名由实际部署替换）：

```yaml
global:
  scrape_interval: 15s
  scrape_timeout: 5s
scrape_configs:
  - job_name: order-api
    static_configs:
      - targets: ["order-api:9091"]
        labels:
          service: order-api
          env: test
  - job_name: order-worker
    static_configs:
      - targets: ["order-worker:9092"]
        labels:
          service: order-worker
          env: test
```

启用并完成实施后，本机验证命令：

```bash
curl -fsS --max-time 5 http://127.0.0.1:9091/metrics
curl -fsS --max-time 5 http://127.0.0.1:8080/ \
  -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'
```

预期第二个请求的响应关联头及日志 trace_id 与传入 TraceID 一致，并在追踪后端查到 server span；仅 curl 成功不等于导出成功。跨服务联调须通过实际受 instrumentation 管理的 HTTP/gRPC 客户端与消息生产端验证。

### 10. 验证与验收矩阵

| 验证项 | 通过条件 |
| --- | --- |
| 四种开关组合 | 都关、只 metrics、只 tracing、都开；关闭功能无额外监听/远程连接，业务可用 |
| Registry 隔离 | 多实例/重复测试无全局注册污染，重复 Collector 注册返回可诊断错误 |
| HTTP/gRPC 指标 | 成功、响应后 panic、取消不写响应、已注册 RPC 解码失败均按真实语义计数；inflight 回到零；health/未知 RPC 被排除且不泄漏 span；未知 HTTP 路径不扩大基数 |
| 运行时指标 | `/metrics` 可解析且含 Go/进程指标；具体 OS 支持差异单列验证 |
| W3C 与旧请求标识 | 合法/非法/缺失/冲突输入、旧 B3 单 ID、采样与不采样、关闭追踪均符合第 4 节；关闭时不把远端 span_id 当作本地 ID |
| 同步链路 | HTTP → gRPC 或 HTTP → HTTP 的 parent/child 正确，日志与响应 ID 一致，无重复 server span |
| Kafka | 生产 → 回调链路连通；重试/重投递不复用 span；超时后返回 nil 仍标错；非法 header 不影响消费；忽略 Run 父 span；提交与顺序语义不变 |
| panic 与敏感数据 | span 正常结束且标错，原有 recovery/重试语义不变；属性不含正文、凭据、错误原文 |
| 采样与后端故障 | ratio 0/1 和父采样决策正确；Collector 不可达不影响业务期限；队列有界 |
| 生命周期 | 端口冲突、启动中途失败、Serve 失败、flush 超时、二次信号及总预算校验可验证；本次新增内部任务退出，不承诺旧心跳被改造 |
| 端到端 | Prometheus 目标 up=1，业务调用后指标变化；OTLP 接收端与追踪后端可检索预期链路 |

单元测试使用独立 Registry、内存 span exporter 和本地 OTLP 替身，不依赖真实观测平台。集成测试按仓库隔离约定执行；HTTP/gRPC/Kafka 热点路径增加观测开关对比 benchmark（耗时、分配及采样影响）。涉及共享基础组件，实施验收需全量回归与人工确认。

### 11. 官方依据与待定项

- [Prometheus 抓取配置](https://prometheus.io/docs/prometheus/latest/configuration/configuration/)
- [Prometheus 埋点建议](https://prometheus.io/docs/practices/instrumentation/)
- [OpenTelemetry Go instrumentation](https://opentelemetry.io/docs/languages/go/instrumentation/)
- [Go 采样](https://opentelemetry.io/docs/languages/go/sampling/)
- [otelgrpc StatsHandler](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc)
- [otelhttp 出站调用](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp)
- [gRPC stats Begin/End](https://pkg.go.dev/google.golang.org/grpc/stats)
- [OTLP HTTP exporter 参数与环境变量优先级](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp)
- [No-op tracing 上下文行为](https://pkg.go.dev/go.opentelemetry.io/otel/trace/noop)

上面的协议、默认值、包边界与故障策略是本项目拟定决策，不是官方强制要求。部署端点、后端产品及生产采样率待实施环境确定；认证/mTLS 不在首版覆盖范围。设计阶段不要求部署平台已就绪，但实现前必须锁定兼容版本并验证固定 exporter 参数、StatsHandler 组合和敏感字段出口处理。请求标识语义已按第 4 节获用户确认，不再维护第二套 legacy_request_id。

## 状态勾点（关键里程碑）

- [x] 明确 Prometheus 最小范围与追加链路追踪范围
- [x] 完成配置、传播、日志关联、生命周期与测试设计
- [x] 核对本地引用、Bash 示例语法和文档差异
- [x] 对照现有代码完成遗漏、范围与冲突复核
- [x] 用户确认请求标识语义与设计方案
- [x] 实施及单元/集成/性能验证
- [x] 验收与归档

## 验收结论

用户已确认本次实现与简化复核，代码验收通过并归档。部署环境验收不在本次代码交付范围，仍按下述边界执行。

| 检查 | 结果 |
| --- | --- |
| 全量测试、全部入口构建、go vet | 通过；真实 Kafka 测试使用隔离主题/组并清理 |
| Linux `go test -race ./...` | 通过，包含信号退出测试；最后监控路径收紧另通过相关单元回归 |
| 独立 SDK test/vet 与外部客户端回归 | 通过；SDK 未引入遥测依赖 |
| 核心加权覆盖率 | 全量检查时 91.67%，排除入口工具、生成/DTO 与可删除 example；telemetry 最终 95.4%，httpmd/requestctx 100%，bootstrap 86.4% |
| Staticcheck v0.8.1 | 两条存量 ST1005：bootstrap/kafka.go 的 Kafka Run panic、lifecycle.go 的 Kafka worker 意外停止；HEAD 干净检出复现相同两条，无新增告警 |
| 文档 | 本地链接、Bash 命令风格复核及 git diff --check 通过（本机未提供 Bash，未执行 bash -n）；配置示例可被 Git 正常收录 |

已覆盖真实 HTTP → gRPC 父子关系及响应关联、真实 Kafka 生产消费与提交恢复、已注册 RPC 的线协议解码失败与流取消、未知方法/health 排除、HTTP 响应写出后 panic、四种开关组合、Registry 隔离及重复注册处理、监控绑定/Serve 失败、采样父决策、队列有界和关闭超时。实际 OTLP 请求验证环境属性/认证头不泄漏，并覆盖官方 HTTP 客户端产生的敏感 URL；TLS 替身验证环境 insecure 配置不能绕过证书校验。

热点 benchmark 已覆盖关闭、启用未采样、启用采样三种模式。单次 200ms 冒烟结果（ns/op，依次为三种模式）：HTTP 3817/3797/5379，gRPC 366/1369/2270，Kafka 213/690/1542；分配分别为 29/36/39、8/24/29、5/13/16 次。测试使用无网络 exporter，gRPC 关闭访问日志输出；这些结果仅用于确认开销与分配可测，不作为生产吞吐承诺。

部署侧仍需人工验证 Prometheus target up=1 以及真实追踪后端可检索链路。本轮不部署或修改观测平台；真实 OTLP 测试使用本地协议替身。未配置隔离 MySQL DSN，相关存储集成测试按既有契约跳过；未执行 Docker 故障注入专项。已核对 Go 构建镜像存在公共 CA 文件，并在 Dockerfile 中复制该文件；未重新构建完整运行镜像。认证/mTLS、自动存储 span、告警平台仍不在首版范围。

## 装配调整验证

简化复核未发现遗漏调用点或传播、降级逻辑回归。补充 gRPC client/server span 直接父子关系和关闭态保留父 SpanID 的断言；配置非法或预算不足降级后，通过实际普通 HTTP 路由验证业务响应及 W3C TraceID 透传。全量 `go test ./... -count=1`、入口构建、go vet 与差异检查通过；本次未执行 race、外部 Kafka/MySQL 集成及 Staticcheck。未修改生产逻辑，该阶段验证完成。

三处封装简化后，全量 `go test ./...`、`go build ./cmd/...`、`go vet ./...` 及差异检查通过。原无观测参数的测试改为显式传入关闭态 Runtime；gRPC 开关两种场景补充旧传播字段清理及调用方 metadata 不变断言，复用现有真实协议调用测试。bootstrap 包覆盖率 89.5%，telemetry 包 95.7%。本次未复跑 race、外部 Kafka/MySQL 集成及 Staticcheck；该阶段验证完成。

心跳配置简化后，删除独立配置结构体及解析函数，改由 bootstrap 直接读取 Viper。新增测试覆盖默认关闭、非法开关、关闭时忽略间隔、无效/非正/溢出间隔降级，以及子进程内正常启用心跳；生命周期配置测试确认 API/worker 不校验心跳。全量 `go test ./...`、`go build ./cmd/...` 和 `go vet ./...` 通过。本次未复跑 race、外部服务集成和 Staticcheck；main 未改动，该阶段验证完成。

本轮按用户最终范围保留 main 的显式业务装配，仅收敛观测全流程，删除 RunAPI/RunWorker。全量 `go test ./...`、`go build ./cmd/...`、`go vet ./...` 通过。新增用例覆盖非法观测/心跳配置、预算不足、初始化失败、端口占用保留追踪、Serve 故障观察、幂等关闭、追踪关闭失败降级以及 Kafka Collector 重复注册后继续消费。

Linux 信号测试增加可选观测失败后 worker 正常退出场景，但本轮 Docker 执行未获授权，未复跑 Linux race/信号测试；本轮未复跑依赖外部服务的 Kafka/MySQL 集成，未重新执行 Staticcheck。前一节历史验证结果不代表本次调整已通过这些检查。该阶段的验证边界保留，代码现已获用户确认。

## 本轮故障隔离复核

已修复三个直接相关的问题：监控抢占未来业务端口；监听主机名解析没有显式期限；退出预算接近耗尽时可选 flush 可能触发总退出 watchdog，同时 SDK 的已取消关闭路径可能漏停批处理器。观测故障降级仍限于配置、监听、导出和关闭等运行故障，不承诺屏蔽任意程序缺陷、进程资源耗尽或必需日志系统阻塞。

两个 main 装配核对无遗漏：API 先校验业务配置、初始化日志和存储，再构造可选观测并显式注入 HTTP/gRPC；worker 校验共享/Kafka 配置、初始化日志，按 Kafka 开关计算排空预算，构造观测并注入消费者。业务 Done 继续接入退出等待，监控 Done 不接入业务退出；关闭按业务、存储（API）、观测、日志执行。

全量测试、入口构建、go vet、git diff --check 通过。bootstrap 覆盖率 89.1%，telemetry 95.5%。新增实际 socket 测试验证 HTTP/gRPC 端口不会被监控抢占、监控 Serve 故障后业务健康接口仍可用；另验证监听取消、日志关闭预算预留、已取消 context 下批处理器仍收到停止通知。本轮未复跑 Linux race/信号和外部存储/Kafka 集成，保留此前授权边界。代码现已获用户确认；部署验证仍由实际部署环境完成。

Staticcheck 沙箱内未能加载包，沙箱外复核仅报告两条已在 HEAD 干净检出确认过的存量 ST1005（Kafka 错误文本首字母），没有新增告警。
