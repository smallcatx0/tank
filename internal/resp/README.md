# internal/resp 包使用说明

本包提供 HTTP 响应、业务错误与分页契约，统一响应体为 `{errcode, msg, data, request_id}`。使用入口为 `Succ`、`Fail`、`Paginate`；调用 `Fail` 后必须返回。

## 错误处理

| 情况 | HTTP / 业务码 | 文案与行为 |
| --- | --- | --- |
| 成功 | 200 / 0 | 操作成功 |
| 参数错误 | 400 / 10000 | 简洁的参数提示 |
| 普通系统错误，env=dev | 500 / 50000 | err.Error()，包括已有包装上下文，方便联调 |
| 普通系统错误，非 dev | 500 / 50000 | 服务错误 |
| context.DeadlineExceeded，含包装 | 504 / 50400 | 请求处理超时 |
| nil error 或空 *Exception | 500 / 50000 | 服务错误 |
| *Exception，含包装 | 对象定义的码值 | 对象定义的安全文案及数据 |
| 请求 context 已取消 | 不再写响应 | 中止后续响应处理 |

`Exception` 通过指针实现 error，`errors.As` 支持 `%w` 包装。`Warp` 是存量预留字段，未实现 Unwrap。预置异常为共享对象，不得修改字段；带业务数据时新建异常。

普通系统错误在 dev 和非 dev 都通过 zap.Error(err) 记录完整错误日志；日志字段策略见 [日志说明](../../pkg/logger/README.md)。dev 回显不会新增堆栈输出；密码、token 等敏感值仍应在构造错误前脱敏。HTTP panic 恢复固定返回服务错误，不回显 panic 值。

## 请求标识

- 请求头 `x-b3-traceid` 接受非全零的 16/32 位十六进制值，统一小写；缺失或非法时生成新的 32 位值。不兼容旧 `request_id` 请求头。
- Trace 同时写入 Gin 上下文 `resp.RequestIDKey` 和标准请求 context。业务层通过 `requestctx.TraceID(ctx)` 读取。
- 响应头 `x-b3-traceid`、响应体 `request_id`、日志 `trace_id` 保持一致。
- 中间件顺序为 Trace → ReqLog → Recovery，完成日志可以取得最终错误或 panic 状态。

## 分页与原始 JSON

`NewPage(c)` 解析 page/limit，`NewPagination` 将 page 归一到至少 1，limit 默认 10、上限 1000。`Calc` 写入总数并钳制越界页码。`Paginate(tx)` 执行 Count，再添加 Offset/Limit；调用方负责绑定 context、确定排序及执行 Find。

`resp.Paginate` 返回 `data.page` 与 `data.list`。店铺示例对空集合传入非 nil 空切片，输出 `[]`。完整实现见 [模型](../../models/dao/schema/shop.go)、[控制器](../../controller/v1/shop.go)，接入步骤见 [业务接入指南](../../业务接入指南.md)。

`SuccJsonRaw(c, data)` 原样嵌入合法 JSON 值，不解析或校验；调用方必须保证合法性，无数据应传 `"null"`。request_id 独立 JSON 编码。

## 追踪开启后的请求标识

响应结构和 `x-b3-traceid` 头名称保持不变。开启追踪时，`request_id`、该响应头与日志 trace_id 统一为当前 OTel 的 32 位 TraceID；仅传旧 B3 单 ID 不再保证回显，合法 W3C traceparent 才能延续父链路。关闭追踪时 W3C 优先，否则保持原 B3 校验与生成行为。详见 [传播契约](../telemetry/README.md)。
