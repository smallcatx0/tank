<!-- D22 | 状态：已完成 -->
# Kafka 消费能力封装

## 问题

脚手架需要提供可复用的 Kafka 消费组件，统一消息处理、位点提交、失败重试与资源释放的契约，避免使用方重复实现消费循环。需求入口见 [plan.md](../../plan.md)。

用户已确认组件位于 `pkg/kafka`，底层采用 franz-go；第一版支持单条消息回调，业务重试耗尽后停止消费者。用户已授权按本文实施，并明确面向较新 Kafka 版本的通用设计。组件、单元测试、真实 Broker 集成测试与使用说明已实现。

编号依据立项时计划中最近的 D21 递增为 D22。用户已确认验收通过，本文按收尾流程归档。

## 规则与约束

- 工程规则引用 [项目规范](../../项目规范.md)，重点遵循第 1、2、5、6、7、8、9、10 节。
- 文档生命周期与信息归属引用 [AGENTS.md](../../AGENTS.md#文档与进度管理强制)。
- 文档结构采用 [设计文档模板](../设计文档模板.md)。
- 日志接入引用 [日志组件说明](../../pkg/logger/README.md)。

## 方案与决策记录

### 1. 边界与第一版范围

- `pkg/kafka` 负责客户端构造、消费调度、单条回调、业务重试、提交、再均衡协调与关闭，不读取项目配置、不引用 `internal/` 或业务包。
- `internal/bootstrap` 后续负责配置转换、注册业务 Handler、启动和进程退出编排；本轮不修改。
- 业务负责消息解析、校验、幂等和副作用。至少一次语义允许重复处理，不承诺数据库与 Kafka offset 的原子事务。
- 一实例绑定一个 GroupID 和一个或多个 Topic，使用普通消费者组。暂停/恢复、动态订阅、手动分区分配、批量 Handler、自动死信、事务消费、同分区并行均不在第一版范围。
- 客户端使用 `github.com/twmb/franz-go/pkg/kgo`，锁定 franz-go v1.22.2、kmsg v1.15.0，兼容仓库 Go 1.26；本地验证 Broker 为 Kafka 4.1.1。
- 仅实现这一种后端，不建立多客户端适配框架，不透传任意 `kgo.Opt`。

### 2. 对外接口

| 接口契约 | 行为 |
| --- | --- |
| `DefaultOptions() Options` | 返回下文明确的默认值；必填字段保持未设置 |
| `NewConsumer(options Options) (*Consumer, error)` | 校验、复制配置并创建客户端；不执行 Handler，不保证网络连接、入组或分区就绪 |
| `(*Consumer).Run(ctx context.Context, handler Handler) error` | 阻塞运行；同一实例只允许一次有效启动；负责运行结束后的客户端清理 |
| `(*Consumer).Shutdown(ctx context.Context) error` | 请求优雅关闭并等待；必须提供带 deadline 的 context；可并发及重复调用 |
| `Handler(context.Context, Message) error` | 同步处理一条消息，nil 表示业务副作用已完成，可提交位点 |
| `Permanent(err error) error` | 为业务错误加不可重试标记，保留 Unwrap；nil 输入返回 nil，重复包装语义不变 |

不暴露 Start、Ack、Commit、Close、底层 Client。消费者运行和进程退出是不同职责，包内禁止 os.Exit、自动重建或自动重启 Run。

`Run` 传入 nil Handler、nil context 时返回参数错误，不启动；重复或并发有效调用 Run 返回 `ErrAlreadyRun`。实例关闭后调用 Run 返回 `ErrClosed`。上述错误使用 errors.Is 判断。未运行过的实例也可以 Shutdown，用于释放构造成功但未启动的资源。

`Run` 正常优雅结束返回 nil；主动取消返回 context 错误；运行失败返回可解包的原因。处理失败使用 `ConsumeError` 携带 Stage（handle/commit/fetch/group/close）、Topic、Partition、Offset、Attempts 和 Err；无具体消息时定位字段不适用。处理 panic 转为错误并记录堆栈。多个终止原因保留首先触发停止的根因，后续独立清理失败合并返回，不用派生 context canceled 覆盖根因。

### 3. 消息契约

| 字段 | 设计类型与语义 |
| --- | --- |
| Topic | string，消息所属 Topic |
| Partition | int32，分区编号 |
| Offset | int64，当前消息位点，不要求相邻记录 offset 数值连续 |
| Key、Value | []byte，原始数据，保留 nil 与空值语义 |
| Headers | []Header；Header 含 Key string、Value []byte，保留顺序与重名字段 |
| Timestamp | time.Time，Kafka 消息时间戳，不冒充本地接收时间 |

Message 与其切片在 Handler 中只读；重试传递相同消息数据。调用方若需在回调返回后使用数据，必须自行复制。Handler 不得在返回 nil 后继续异步处理消息。并发上限大于 1 时，Handler 必须支持不同分区的并发调用。组件不推断业务 ID，也不默认输出消息正文或认证信息。

### 4. 配置与默认值

配置通过 Options 显式传入，不读取 YAML、环境变量或全局配置。DefaultOptions 是默认值唯一来源；NewConsumer 不把用户显式填入的非法零值偷偷替换成默认值。所有数值按下表校验，运算检查溢出。这里的默认值是第一版设计基线，尚未经过生产压测。

#### 4.1 基础与业务处理

| 字段 | 默认值 | 校验与语义 |
| --- | --- | --- |
| Brokers | 无，必填 | 非空地址列表，元素非空 |
| GroupID | 无，必填 | 非空 |
| Topics | 无，必填 | 非空列表，元素非空且不重复；不支持正则订阅 |
| ClientID | gtank-consumer | 非空；允许由装配层覆盖 |
| StartOffset | 未设置，必填 | 仅 earliest/latest；仅在没有有效提交位点时使用 |
| MaxConcurrency | 1 | 正整数，限制整条消息处理及提交占用的工作槽 |
| AttemptTimeout | 10s | 正数，单次 Handler 期限，不含退避与提交 |
| MessageTimeout | 30s | 正数且不小于 AttemptTimeout；从第一次调用开始计时，含全部尝试和退避，不含排队和提交 |
| Retry.MaxAttempts | 3 | 总调用次数，包含首次；最小 1 |
| Retry.InitialBackoff | 200ms | 正数 |
| Retry.MaxBackoff | 2s | 不小于 InitialBackoff |
| CommitTimeout | 5s | 正数，从获取提交入口前开始计时，含等待、网络与内部重试 |
| TLS | nil | nil 表示不启用；非 nil 使用标准 tls.Config 副本，最低 TLS 1.2，禁止跳过服务端验证 |
| SASL | 无 | 支持 none、plain、scram-sha-256、scram-sha-512；启用时用户名与密码必填，要求 TLS；第一版不做 OAuth/Kerberos |

不提供 CommitMode、AutoCommit、CommitInterval、ShutdownTimeout、失败后跳过开关。永久错误通过 Permanent 表达，不引入返回状态枚举。

#### 4.2 Advanced：传输、拉取与消费者组

| 字段 | 默认值 | 行为及底层映射 |
| --- | --- | --- |
| DialTimeout | 5s | 连接期限，对应 kgo.DialTimeout |
| RequestTimeoutOverhead | 5s | 对应同名选项；是请求协议等待之外的附加期限，不宣称所有请求总耗时只有 5s |
| RequestRetryTimeout | 30s | 普通请求重试总预算；组请求使用 Group.RebalanceTimeout，提交另受 CommitTimeout 约束；不作用于 fetch |
| RequestMaxAttempts | 3 | 可重试请求总尝试上限，映射 kgo.RequestRetries；不包含业务 Handler 重试 |
| PollTimeout | 1s | 每次 PollRecords 等待期限；仅轮询超时且没有数据/其他错误时继续，不当成业务失败 |
| MaxPollRecords | 1 | 正整数；每轮交给应用的消息总上限，所有分区合计 |
| FetchMinBytes | 1 B | 正数 |
| FetchMaxBytes | 1 MiB | 单次 broker fetch 目标上限，且不小于 FetchMaxPartitionBytes |
| FetchMaxPartitionBytes | 1 MiB | 单分区 fetch 目标上限 |
| FetchMaxWait | 500ms | broker 等待最低字节数的期限，正数 |
| MaxConcurrentFetches | 1 | 正整数，限制底层同时进行的 fetch |
| BrokerMaxReadBytes | 16 MiB | 响应大小保护，应大于 FetchMaxBytes，并与集群最大批次协调 |
| MaxDecompressBatchBytes | 64 MiB | 显式限制单批解压大小，映射同名选项；正数且不超过 MaxInt32 |
| Group.SessionTimeout | 45s | 心跳失联期限，须在 broker 允许范围内 |
| Group.HeartbeatInterval | 3s | 正数，至多 SessionTimeout 的三分之一 |
| Group.RebalanceTimeout | 60s | 再均衡等待预算，必须满足第 6 节校验 |

底层固定使用 classic 消费者组、cooperative-sticky 分配策略、DisableAutoCommit 和 BlockRebalanceOnPoll；不启用静态成员、下一代组协议或自动创建 Topic。第一版固定 read_committed，避免把已中止事务的数据交给业务，但不因此宣称业务处理 exactly-once。若接入已有消费者组，需要先核对其分配策略兼容性。

客户端请求退避显式设置为指数退避，基数 250ms、上限 2s，并使用与业务相同的有界随机抖动算法。没有针对提交再套一层重试循环；SDK 的尝试次数与各自 deadline 共同约束。普通重连、重新入组由客户端维护，其多次恢复循环不等于一次请求的无限重试；无消息不被视为故障。已经暴露给组件的不可恢复认证、授权、组错误及非预期 fetch 错误结束 Run，正常组迁移由客户端处理。

预取限制不等于进程内存硬上限：Broker 为推进消费可能返回大于目标 fetch 大小的批次，解压后也可能放大。应用层不再建立跨轮队列；底层读取保护、fetch 并发与指标共同控制资源。实现时需针对锁定版本验证解压保护能力与默认限制，禁止用 MaxPollRecords 推导总内存上限。

### 5. 提交与重试

#### 5.1 固定逐条同步提交

处理顺序为「Handler 成功 → CommitRecords 成功 → 同分区下一条」。不同分区的处理可以并行；通过一个可取消等待的提交入口串行执行同步提交，每次只提交当前已成功消息，避免并发提交的次序管理复杂化。CommitTimeout 包含等待该入口的时间。

offset 提交表示下一次读取的位置；由 CommitRecords 转换，封装不额外加一。禁止 CommitUncommittedOffsets 或在退出时提交所有已拉取位点。前序已投递记录未成功时，不提交同分区后续记录；连续成功指消费顺序连续，而非 offset 数值必须逐一加一。

提交失败只重试提交，不再次执行已成功 Handler。最终提交错误停止整个实例。取消或网络超时后不能撤销已发出的提交，结果可能不确定；重启可能重复处理。失败消息、未开始消息和取消后才完成的业务结果都不产生新的提交请求。

#### 5.2 业务重试

- 普通错误：在 MaxAttempts 与 MessageTimeout 的双重预算内重试同一条消息。
- 第 n 次重试（n 从 1 开始）的基准等待为 min(MaxBackoff, InitialBackoff × 2^(n-1))；实际等待在基准的 80% 至 100% 间随机取值，避免超出 MaxBackoff。指数计算须饱和，不能整数溢出。
- Permanent、panic：不重试，触发全实例失败停止。
- 单次超时：即使 Handler 随后返回 nil，也按该次失败处理；仅在旧调用已经返回且整条预算仍有效时重试。不得用遗留 goroutine 模拟强制超时。
- 运行取消、强制停止、失去分区所有权：取消相关任务，不进行业务重试；取消后的返回结果不能触发提交。
- 重试耗尽或整条期限耗尽：返回带定位信息、尝试次数和根因的错误，停止全部分区。
- 重启时尝试次数重新计算，不持久化重试次数；持续失败消息可能反复导致进程退出，修复/人工处置属于业务运维职责。

### 6. 内部消费调度

#### 6.1 有界轮次

Run 是唯一 poll 协调者；一个非空 poll 返回后，在本轮结束前不再 poll。每轮至多 MaxPollRecords 条，按 Topic/Partition 划分有序队列，每个分区同时至多一个任务。以 errgroup.WithContext 与 SetLimit(MaxConcurrency) 执行本轮分区任务；一个分区任务逐条处理其本轮记录，逐条同步提交。

每条新消息进入 Handler 前，在与 Shutdown 共用的状态锁下检查并登记为在途；这一步是「已开始」的线性化边界。收到关闭后，等待工作槽或已分组但未登记的消息均不再执行。禁止将整个分区队列一次性标记为在途。

业务处理、重试等待与提交均占用工作槽；慢分区会占用一个槽，但不会在有其他空闲槽时阻止其他分区执行。受轮次屏障限制，本轮慢任务仍会延后下一轮，这是第一版明确接受的吞吐代价，不承诺分区间完全隔离。

本轮全部工作结束后，协调者释放消息引用，调用 AllowRebalance，再进入下一轮。默认 MaxPollRecords=1、MaxConcurrency=1，效果为全实例逐条处理；仅提高 MaxConcurrency 不会提高有效并发，需要同时调整拉取条数和预算。非空结果伴随致命 fetch 错误时不派发该轮消息，直接失败停止。

#### 6.2 再均衡预算

以所有记录都来自同一分区的最坏情况校验，不能简单用记录数除以并发数：

```text
N = Advanced.MaxPollRecords
T = MessageTimeout + CommitTimeout
Reserve = 5s（内部固定协调余量）
要求：N × T + Reserve < Group.RebalanceTimeout
```

默认值为 1 × (30s + 5s) + 5s = 40s，小于 60s。构造阶段拒绝不满足预算或发生溢出的配置，不静默修改用户超时。增加 N 时必须缩短单条预算或增大再均衡预算；例如 N=4、MessageTimeout=5s、AttemptTimeout=2s、CommitTimeout=2s、RebalanceTimeout=60s 满足校验。

每轮另设总 deadline 为 N × T，防止协作正常的任务累计超时。轮次总 deadline 不因业务重试或 Shutdown 重置。PollTimeout 不计入已取得消息后的处理预算；请求等待和提交入口等待已计入各自期限。

该校验是调度预算，不是分区所有权保证；网络故障、进程停顿、心跳中断仍可能导致移出组。Handler 不响应 context 时也无法硬性兑现预算，不能宣称绝不会再均衡或重复。

#### 6.3 分区撤销与丢失

BlockRebalanceOnPoll 用于保证正常情况下完成本轮处理和提交后才允许撤销回调。Assigned/Revoked/Lost 回调仅更新归属状态和日志，不启动业务，不提交已拉取记录。

正常撤销发生在本轮工作结束并 AllowRebalance 后；Lost 或已知组成员失效时禁止发起新提交。由于阻塞模式可能延迟回调，不能依赖回调及时发现所有权丢失；提交返回的组错误同样触发失败停止。任何未知结果均按可能重复处理解释，不试图用重新入组后提交旧消息来补偿。

### 7. 生命周期与优雅关闭

状态只有 created、running、draining、stopping、closed，状态转换单向。draining 表示停止新派发但允许已开始消息完成；stopping 表示取消在途处理与提交；closed 只在任务退出且客户端关闭完成后设置。

| 触发 | 动作 | 结果 |
| --- | --- | --- |
| 正常 Shutdown | running → draining，停止 poll/新派发，在原有预算与关闭期限内完成在途消息 | 成功清理后 Run、Shutdown 均返回 nil |
| Run ctx 取消 | → stopping，取消任务，不继续业务重试 | Run 返回可通过 errors.Is 判断的 context 错误 |
| 业务/提交/致命客户端失败 | → stopping，停止全实例，取消其他在途任务 | Run 返回首先触发停止的根因及独立清理错误 |
| 排空期限到达 | draining → stopping，取消任务与提交 | Shutdown 返回超时，Run 待实际清理结束后返回超时原因 |
| created 状态 Shutdown | 不启动 Run，直接关闭客户端 | 关闭成功后为 closed |

正常关闭具体顺序：停止派发 → 在途消息处理及同步提交 → 等待本轮任务退出 → 允许再均衡并关闭客户端 → 发布关闭完成结果。客户端关闭路径必须使用与 BlockRebalanceOnPoll 配套的释放方式（如 CloseAllowingRebalance），避免关闭等待被自身阻塞。没有额外「全量最终提交」步骤。

Shutdown 必须使用独立且带 deadline 的 context，不能使用已经取消的 Run context。多个并发调用共享同一个关闭过程，采用最早的已登记关闭期限；任何调用取消或期限到达都会升级为 stopping，后续调用不能重新延长排空期限。对已经 closed 的实例，重复 Shutdown 返回已保存的关闭结果。Shutdown 返回运行根因与清理错误，正常退出时为 nil。

Shutdown 超时仅表示本次排空失败，不表示所有 goroutine 或客户端已经释放。组件不把仍执行的 Handler 遗留到后台后假装关闭成功；Run 继续负责收敛与清理。若 Handler 永不响应取消，Run 可能无法返回，由进程的总退出期限/二次信号强退兜底。库内不使用 os.Exit。

消息在正常排空过程中仍可按原预算重试；预取但未开始的消息一律丢弃本地引用，不执行、不提交。进入 stopping 之前已发出的提交可能成功，不能回滚；进入 stopping 后不接受新的提交。

### 8. 可观测性与装配

包内通过 pkg/logger 接入 zap，客户端日志适配到同一入口，不创建独立日志系统。日志定位包含 consumer/group/topic/partition/offset、attempt、stage、latency_ms，错误与 panic 记录原因和堆栈；业务 trace_id、业务 ID 由可识别的业务上下文提供，不虚构值、不默认解析正文。

第一版监测处理成功/失败/重试计数、处理及提交耗时、在途消息数量、应用待处理数量、关闭失败及再均衡次数。Prometheus 接入须由装配提供注册位置，禁止构造消费者时向默认全局注册器重复注册；确切指标接入接口随实现前的指标设施核对确定，不影响消费与提交契约。offset、消息 Key、错误原文不作为指标标签。

后续装配须使现有退出协调器能够监听 Consumer.Run 的结束结果；消费者先于数据库、Redis 与日志关闭。运行失败是否导致进程退出由装配层决定。进程全局退出预算必须覆盖消费者排空与后续资源关闭；不足以完成 MessageTimeout 的退出预算允许发生取消与重复消费，不自动扩大进程退出预算。

### 9. 实现前核对与验证计划

用户已授权实现，环境采用 Kafka 4.1.1、PLAINTEXT、新建 classic/cooperative-sticky 消费者组。生产认证方式、已有组策略和最大消息批次未限定；具体生产环境需按配置契约核对。它们是环境兼容性核对项，不改变已确认的提交、重试和关闭规则。

| 验证维度 | 必须覆盖的行为 |
| --- | --- |
| 配置 | 默认值、必填、非法零值、超时预算、溢出、TLS/SASL 组合 |
| 顺序与提交 | 同分区不并行，失败不越过，offset 有空洞，提交失败不重跑 Handler，不提交预取消息 |
| 重试 | 次数含首次、退避边界、整条超时、Permanent、panic、单次超时后迟到 nil、重启计数重置 |
| 并发 | 全局并发上限、工作槽等待、逐条提交串行化、失败取消其他任务、Shutdown/派发竞态 |
| 生命周期 | 未 Run 关闭、重复 Run、重复/并发 Shutdown、最早期限、运行取消、关闭超时后真实完成、不响应取消的 Handler |
| 真实 Kafka 集成 | 两消费者再均衡、撤销与失联、提交响应丢失/超时、认证失败、Broker 恢复、已提交/首次位点、事务消息隔离 |
| 资源与性能 | 应用待处理数量有界、底层预取与压缩大批次内存、goroutine 回收、同步提交吞吐 benchmark |

测试结果和未覆盖的环境场景见下方验收结论；不将计划中的全部测试视为已经通过。

### 10. 实现补充决策

- 仓库尚无 Prometheus 注册入口。Consumer 实现 `prometheus.Collector`，由调用方向自己的注册器注册；不新增默认全局注册、HTTP 指标路由或配置读取。指标以 consumer/group 为固定标签，以有界 event/stage 为可变标签。
- SDK 源码核对：`RequestRetries` 表示总尝试次数，直接映射 RequestMaxAttempts；组请求 11/12/14 的重试预算使用 RebalanceTimeout。未启用 ServerSideBalancer，保持 classic 协议；`GroupProtocol` 是 JoinGroup 的协议类型，不误设为字符串 classic。
- SDK 的默认单批解压上限为 1 GiB。本组件显式提供 MaxDecompressBatchBytes，默认 64 MiB，并以真实压缩消息验证拒绝超限批次。该限制不等于进程内存硬上限。
- 通过 PreCommitFnContext 在 SDK 形成提交请求前再次检查取消、停止状态与分区归属；已经发出的请求依然可能成功，不能撤销。
- Shutdown 的每个调用独立监听自己的 context，最早取消或超时触发共享 stopping；回调必须返回后才完成资源回收。Run 业务上下文值向 Handler 传递，排空使用独立的取消链。
- TLS 使用标准 Clone，并将未设置的 MinVersion 提升至 TLS 1.2；Clone 内部引用的证书和回调必须保持只读。panic 记录类型和堆栈，不转储任意 panic 字符串或消息正文。
- 本轮交付组件与可运行接入示例；沿用第 1 节边界，不接入现有 bootstrap/业务入口，不虚构业务 Handler。用法及持久本机 Kafka 管理见 [组件说明](../../pkg/kafka/README.md)。
- 真实认证验证发现 SDK 的连接初始化错误不一定出现在 PollRecords 中。新增 HookBrokerConnect 接收不可恢复的 TLS 证书校验与 SASL 认证错误；普通 EOF、连接超时不由该 Hook 终止消费者。新增 HookGroupManageError 保留组授权/配置错误，并在已有分区的组会话终止时及时取消在途处理，避免等待受 BlockRebalanceOnPoll 延迟的 Lost 回调。
- 连接错误可能早于首次 Run。此时保存根因并取消内部 context，保留 created 的清理所有权，由随后第一次 Run 或 Shutdown 关闭客户端；不得误报 ErrAlreadyRun 或遗留客户端。回调只发布停止信号，不在 SDK 回调锁内关闭客户端。
- 专项测试使用自有 Kafka 4.1.1 容器和按 ClientID/correlation ID 定位的协议代理。丢失请求或响应后将该 TCP 连接后续流量一并丢弃，模拟 TCP 黑洞而非伪造越序响应；普通连接中断单独验证 SDK 可重新连接。测试证书和凭据每次生成，结束清理；常驻 Kafka 的部署操作不属于本套件。

### 11. 官方资料

- [franz-go 消费指南](https://github.com/twmb/franz-go/blob/master/docs/producing-and-consuming.md)：消费组、再均衡与手动提交的背景依据。
- [kgo API 文档](https://pkg.go.dev/github.com/twmb/franz-go/pkg/kgo)：提交、阻塞再均衡、关闭、fetch、请求期限与消费者组选项。
- [Kafka 4.1 SASL](https://kafka.apache.org/41/security/authentication-using-sasl/) 与 [TLS 配置](https://kafka.apache.org/41/security/encryption-and-authentication-using-ssl/)：专项环境的认证机制、监听器与服务端证书配置依据。

本文的接口、默认值、轮次调度和预算公式是本项目的设计决策，不是对客户端默认行为的照搬。实现时以锁定版本的官方文档和集成验证为准。

## 状态勾点（关键里程碑）

- [x] 用户确认组件位置、客户端、单条回调与重试耗尽停止策略。
- [x] 用户确认逐条同步提交、业务重试与优雅关闭规则。
- [x] 整理接口、配置默认值、内部调度与验证计划。
- [x] 用户审阅本版设计细化内容。
- [x] 获得实施指令，完成本地环境兼容性核对、版本锁定与指标接入接口细化。
- [x] 实现组件、接入示例与配套测试；实际业务装配按第 1 节范围留待后续。
- [x] 完成真实认证与网络故障专项验证，并回归已有集成测试。
- [x] 完成人工验收与归档。

## 验收结论

用户已确认 D22 验收通过。组件、真实认证与网络故障专项验证，以及认证根因传递和组会话错误处理修复均已验收，按 AGENTS.md 完成归档；验证结果与适用边界如下。

已验证：

- Windows：全量 `go test ./...`、`go build ./cmd/...`、`go vet ./...`、staticcheck v0.7.0。
- Linux Go 1.26 测试容器：全量 `go test -race ./...`；宿主机无 C 编译器，因此竞态检查在容器中执行。
- Kafka 包单元、常规集成和真实认证/故障专项合计语句覆盖率 95.8%；本次 Hook 修复后重跑全量单元测试、构建、vet、staticcheck 与全量竞态检测，均通过。真实 Docker 专项在 Windows 原生执行；Linux 竞态检查覆盖默认单元测试，不将跳过的 Docker 专项记为竞态验证。
- 单元测试涵盖参数校验、配置复制、TLS/SASL 构造、消息头保真、重试/永久错误/panic、单次超时迟到成功、同分区顺序、提交失败不重跑 Handler、取消与排空、重复/并发关闭、最早关闭期限、在途任务未退出不得提前发布关闭、跨分区失败取消、提交互斥、分区归属与指标注册。
- Kafka 4.1.1 集成测试涵盖双消费者再均衡与撤销、已提交位点续读、earliest/latest 首次位点、失败记录重新投递、read_committed 过滤中止事务、压缩批次解压上限。
- Windows i7-12700K 基准：消息视图与退避约 35 ns/op、48 B/op；单条内存提交替身轮次约 5.1 µs/op、2880 B/op。此结果仅用于组件开销参考。

专项验证结果（`TestDockerAuthenticationAndNetworkFaults`，真实 Kafka 4.1.1）：

| 场景 | 观察与断言 | 结果 |
| --- | --- | --- |
| TLS + PLAIN / SCRAM-SHA-256 / SCRAM-SHA-512 | 各机制均完成消费与真实位点提交；错误密码返回可解包的 SASL_AUTHENTICATION_FAILED，不等待外层超时 | 通过 |
| 未知 CA / 主机名不匹配 | Handler 不执行，Run 保留 x509 校验根因 | 通过 |
| ACL 拒绝 | 有效身份但无资源权限，Run 返回 Topic/GroupAuthorizationFailed，Handler 不执行 | 通过 |
| 普通 TCP 断连 | 同一 Consumer 自动重连继续处理，累计两条记录提交到 offset=2 | 通过 |
| 提交请求丢失 | 代理确认命中 OffsetCommit；Handler 仅一次，提交期限内失败，Broker offset=-1；新实例重新读取 first | 通过 |
| 提交响应丢失 | Broker 已执行提交，代理丢弃 ACK；客户端超时且 Handler 仅一次，Broker offset=1；新实例从 second 继续 | 通过 |
| 真实心跳失联 | 代理丢弃心跳，DescribeGroups 确认 Broker 已移除成员；停止消费，旧消息无提交，offset=-1 | 通过 |
| Broker 强制终止与恢复 | 在 Handler 执行期间终止自有 Broker，后续提交失败；重启后保留日志，未提交记录重投，最终 offset=2 | 通过 |
| 既有集成回归 | 同一独立 Broker 上重跑再均衡、重投、事务隔离、解压限制和 latest 位点测试 | 通过 |

问题归因：修改前 D22 代码的独立副本复现了错误 PLAIN 凭据只能返回 context deadline exceeded 的问题；上述 Hook 修复后保留真实认证根因。另一次混合回归期间常驻 Kafka 被外部部署操作替换，导致依赖常驻端口的旧集成测试断连；专项套件已改为在自有 Broker 上完成这些回归，不修改常驻部署。

验收边界：上述有限故障场景已验证，不代表所有网络分区或多节点故障组合均已覆盖。生产最大批次、证书轮换和已有组兼容性仍属于具体接入环境核对；本地测试不构成生产吞吐或多副本高可用结论。
