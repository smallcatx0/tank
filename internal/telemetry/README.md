# 观测接入

bootstrap 统一装配可选心跳、Prometheus 指标和 OpenTelemetry 追踪，默认分别关闭。main 保留业务装配，通过 `bootstrap.StartObservability(...)` 接入观测；业务传递 `context.Context`，观测资源由 bootstrap 管理。

## 开启与验证

HTTP、gRPC 和 Kafka 启动函数均显式接收一个非 nil 的 `*telemetry.Runtime`，使用 `StartObservability` 返回的 `Runtime`；关闭观测时仍传入关闭态实例，以保留上下文传播。监控监听仅由 bootstrap 内部装配，无需单独调用。

在 [API 配置](../../conf/app.yaml.example)或 [worker 配置](../../conf/worker.yaml.example)中修改 `observability`：

```yaml
observability:
  service_name: order-api
  metrics:
    enable: true
    address: 127.0.0.1:9091
    shutdown_timeout: 2s
  tracing:
    enable: true
    endpoint: http://127.0.0.1:4318/v1/traces
    sample_ratio: 1
    export_timeout: 3s
    shutdown_timeout: 5s
```

同时调整根级 `shutdown_timeout`：API 全开示例为 **40** 秒，worker 为 **55** 秒。worker 同机监控端口改为 9092。启用观测所需预算必须严格大于业务排空之和 + 已开启观测关闭期限 + 5s 清理余量，不足时警告并跳过 metrics/tracing；默认关闭模板仍为 30/45 秒。Docker 停止等待相应设置为 45/60 秒。

容器内供外部抓取时监听 `0.0.0.0`，只向可信监控网络开放监控端口。每个实例独立抓取，不能使用随机负载均衡的业务地址。[Prometheus 抓取示例](../../conf/prometheus.yaml.example)和 [Collector pipeline 示例](../../conf/otel-collector.yaml.example)需替换为部署侧地址；Collector 不提供持久追踪存储。

```bash
curl -fsS --max-time 5 http://127.0.0.1:9091/metrics
curl -fsS --max-time 5 http://127.0.0.1:8080/ \
  -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'

# API 还需开启 grpc.enable；追踪客户端与 API 使用各自的 service_name
go run ./test/grpc_trace -addr 127.0.0.1:9090 -timeout 3s \
  -trace-endpoint http://127.0.0.1:4318/v1/traces

# 只使用独立测试主题；主题必须预先创建，消费者同样开启 tracing
go run ./test/kafka_producer -brokers kafka:9092 -topic example-events \
  -count 1 -trace-endpoint http://collector:4318/v1/traces
```

检查 Prometheus target `up=1`、业务 Counter 增长，并在后端按响应/日志中的 trace_id 查找链路。抓取成功或发送回执成功均不能代替链路检索验收。存量纯 SDK 验证工具仍为 `test/grpc_version`，不依赖遥测基础设施。

## 配置与故障边界

- `metrics.enable/dt` 是原来的系统日志心跳；Prometheus 仅使用 `observability.metrics`。两个观测开关相互独立，关闭项不校验其无关参数。
- `service_name` 仅 tracing 开启时必填；`ver`、`env` 写入资源属性，每次启动生成独立实例 ID。YAML 和固定 SDK 选项控制导出行为，不使用 OTEL 环境变量覆盖应用配置。
- `endpoint` 是完整 HTTP(S) traces URL，禁止内嵌凭据、查询参数或 fragment。跨信任边界用 HTTPS；镜像携带公共 CA，私有 CA 由部署侧注入。首版不提供认证/mTLS 配置。
- `sample_ratio` 为 0..1，仅控制根链路；已采样父链路即使在比例为 0 时仍可被继承。未采样也有有效 ID，不能保证错误链路全部保存。
- 观测配置或初始化失败时警告并降级为关闭态，保留 W3C 透传；监控监听主机名解析和绑定限时 3s；若配置端口与已启用的 HTTP/gRPC 业务端口相同，预先跳过指标，为业务让位。其他端口绑定冲突只关闭指标，保留追踪。监控 Serve 故障、Kafka Collector 注册失败只警告，不触发业务退出或覆盖其他实例的注册。Collector 不可达只限频记录导出失败；队列满允许丢弃 span。
- 心跳由 bootstrap 直接读取 Viper，不保留独立配置结构体。默认关闭，默认间隔 10 秒；采用 Viper 的宽松类型转换，无法转换的开关按关闭处理，启用后间隔无法转换、非正数或超出时长范围时警告并跳过心跳。HTTP/gRPC/Kafka、存储和日志等必需业务配置与启动失败仍快速失败。
- 退出按业务排空 → 存储 → 监控 → 追踪 → 日志顺序执行。观测关闭给总退出期限预留最后 1s 用于日志关闭，预算不足时取消观测等待；关闭失败只警告，不单独改变退出码；追踪 Shutdown 有独立期限，不承诺强退或后端故障时零丢失。原有心跳生命周期没有改造。

## 业务调用

`StartObservability` 返回的 `observation.Runtime` 是 `*telemetry.Runtime`，装配层持有该对象，通过构造参数将已装配客户端传给业务，不在处理器中重复创建连接池。

```go
// 启动装配时：transport 的池大小/网络期限按服务需要显式配置。
client := &http.Client{
    Transport: observation.Runtime.HTTPTransport(transport),
    Timeout:   3 * time.Second,
}
conn, err := grpc.NewClient(address,
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    observation.Runtime.GRPCClientOption(),
)
```

HTTP 请求用 `http.NewRequestWithContext(ctx, ...)`，读完并关闭响应体；RPC 使用 `context.WithTimeout(ctx, ...)`。需要手工 span 时注入 `observation.Runtime.Tracer()`，不要依赖 OTel 全局默认 Provider。Kafka worker 在 main 中保留 `MustKafkaWorker(kafkaCfg, handler, observation.Runtime)`，显式选择业务回调并传入观测对象。

Kafka 生产端调用 `StartProducer` → `InjectKafka` → 发送 → Broker 确认后调用结束函数，完整用法见 [测试生产者](../../test/kafka_producer/main.go)。消费回调每次重试生成新的同级 span；回调成功不表示之后的位点提交成功。包装层不改变原返回值、panic、期限或重试策略。

## 传播与隐私

只传播 W3C `traceparent/tracestate`，不自动传 baggage。开启 tracing 后，HTTP 响应 `request_id`、`x-b3-traceid` 和日志 trace_id 使用当前 32 位 TraceID；只有旧 B3 单 ID 的输入会创建新链路。关闭 tracing 时合法 W3C 优先，否则 HTTP 保留原 B3 关联规则；gRPC/Kafka 不伪造 ID。`requestctx.SpanID` 仅返回本地有效 span ID。

span 只保留受控方法、路由模板、状态和 Kafka 消息位置，不包含正文、完整 URL、query、SQL、错误原文或异常堆栈。SDK 的出口视图固定 Resource 并剔除自由文本状态、事件和 Link；不修改原 span。日志仍遵循 [日志规范](../../pkg/logger/README.md)。不要将秘密放进业务自定义 span 名称。

## 指标语义

| 指标 | 标签/语义 |
| --- | --- |
| `service_http_requests_total` | method、route、status、outcome |
| `service_http_request_duration_seconds` | method、route；完整处理耗时 |
| `service_http_inflight` | 当前业务 HTTP 数，无标签 |
| `service_grpc_requests_total` | service、method、code |
| `service_grpc_request_duration_seconds` | service、method；流式 RPC 统计整条流 |
| `service_grpc_inflight` | 当前已注册业务 RPC 数，无标签 |
| `kafka_consumer_*` | 沿用 [Kafka Collector 契约](../../pkg/kafka/README.md)，pending 不是 lag |

HTTP 未匹配路径为 `unmatched`，非常用方法为 `OTHER`。outcome 按 panic、canceled、deadline_exceeded、server_error、client_error、ok 判定，status 始终保留实际响应状态（响应后 panic 可能仍为 200）。健康路由与抓取请求排除；gRPC 仅记录已注册业务方法，未知方法/握手失败不在统计范围。

每个进程独立 Registry，包含 Go/进程 Collector（进程项依赖 OS 支持）。耗时桶为 5ms 至 10s，不为长流精细分位数作保证。不将 TraceID、用户 ID、消息 key、offset 作为指标标签，也不启用 exemplar。没有自动 SQL/Redis span、Kafka lag、告警或仪表盘部署。
