<!-- D05 | 状态：已完成 -->
# HTTP 响应空错误兜底

## 问题

当前 Response 在 dev 环境对 nil error 调用 Error，在 *Exception 分支解引用空指针，会导致 panic。用户要求修复 nil panic，并明确接受 dev 环境返回原始错误信息。

## 规则与约束

遵循 [仓库协议](../../AGENTS.md) 与 [项目规范](../../项目规范.md)。本次用户明确允许 dev 原始错误响应，作为规范第 2 节外部错误规则的开发环境例外；生产环境保持原行为。

## 方案与决策记录

- Response/Fail 的 nil error 和 typed nil *Exception 均返回 HTTP 500、errcode 50000、msg 服务错误、data null，保留 request_id；成功仍显式调用 Succ。
- 默认初始化失败响应，空错误不调用 Error、不解引用，不新增反射或恢复封装。
- 普通 dev 错误仍返回原始信息，生产环境仍返回通用错误；现有业务异常与成功响应不变。
- 不扩展到任意自定义错误类型内部的 panic，不改包装错误识别、日志装配或其他评审事项。

## 状态勾点（关键里程碑）

- [x] 明确空错误行为与 dev 例外。
- [x] 修复并覆盖 dev/prod 的空错误及现有响应回归。
- [x] 完成针对性验收与收尾。

## 验收结论

2026-11-04：go test -count=1 -cover ./internal/response ./internal/httpmd ./routes 与 go vet ./internal/response 均通过。测试直接调用 Fail/Response，不挂 Recovery，覆盖 dev/prod 的 nil、typed nil *Exception、普通错误、业务异常指针/值及成功响应，核对状态码、错误码、消息、data 和 request_id。

遗留：response 包覆盖率由此前评审的 35% 提升至 70%，仍未达到规范 85% 门槛；本次未扩展到其余既有分页、原始 JSON 等分支。未执行真实存储集成或全量回归，本次未涉及存储或启动装配。
