<!-- D07 | 状态：已完成 -->
# 原始 JSON 成功响应契约

## 问题

SuccJsonRaw 为透传接口避免重复 JSON 解析及序列化，需要保留；现有成功码与 Succ 不一致，request_id 未转义可能破坏外层 JSON。

## 规则与约束

遵循 [仓库协议](../../AGENTS.md) 与 [项目规范](../../项目规范.md)。合法 JSON 由调用方保证，本函数不校验或解析 data。

## 方案与决策记录

- 保留函数签名、HTTP 200、JSON Content-Type 和原始 data 文本，成功码改为 0。
- 仅使用 encoding/json.Marshal 编码 request_id 字符串（该类型不会产生编码错误），将包含引号的编码结果嵌入外层 JSON；不使用 Go 字符串转义代替 JSON 转义。
- data 不解码、不重新编码、不压缩、不校验；调用方必须传入完整合法 JSON 值，空字符串不等价于 JSON null。
- 测试检查特殊请求标识往返、成功契约、对象/数组/标量/null，以及含空白、大整数、转义和百分号的 data 原样保留。

## 状态勾点（关键里程碑）

- [x] 明确透传边界与最小方案。
- [x] 修改实现、测试及使用说明。
- [x] 验证并同步进度、归档。

## 验收结论

2026-11-04：go test -count=1 -cover ./internal/response ./internal/httpmd ./routes 与 go vet ./internal/response 均通过。覆盖成功码、Content-Type、请求标识转义和 JSON data 原始字节保留，并回归既有错误响应测试。

response 包覆盖率为 79.0%（变更前 72.6%），仍有未达 85% 的既有覆盖率缺口；未扩展其他响应或分页功能，未运行全量回归或真实存储集成。
