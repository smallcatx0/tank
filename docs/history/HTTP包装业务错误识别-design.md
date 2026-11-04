<!-- D06 | 状态：已完成 -->
# HTTP 包装业务错误识别

## 问题

Response 仅直接断言 Exception 与 *Exception，业务层通过 fmt.Errorf 的 %w 包装后会丢失业务状态码与错误码，被当成普通 500 错误。用户要求最小修复，保留 dev 原始错误行为。

## 规则与约束

遵循 [仓库协议](../../AGENTS.md) 与 [项目规范](../../项目规范.md)。用户明确保留 dev 普通错误原文响应这一开发环境例外。

## 方案与决策记录

- 在 Response 内使用标准库 errors.As 查找 *Exception 和 Exception，保留 HTTPCode、ErrCode、Msg、Data，不新增封装层或依赖。
- nil error 及匹配到的空 *Exception 仍返回通用 500；支持多层 %w 包装。
- 未匹配业务异常时继续处理传入的完整 error：dev 返回原文（含包装上下文），非 dev 记录错误并返回通用消息。
- 指针异常匹配优先于值异常；不扩展 Exception.Warp 语义或自定义多错误选择策略。

## 状态勾点（关键里程碑）

- [x] 明确识别规则与兼容边界。
- [x] 实现并补充包装错误回归测试。
- [x] 完成验证、同步现状与归档。

## 验收结论

2026-11-04：go test -count=1 -cover ./internal/response ./internal/httpmd ./routes 与 go vet ./internal/response 均通过。覆盖 dev/prod 下直接业务异常、单层和多层包装的指针/值异常、Data 保留、完整普通错误原文、nil 与包装空异常兜底。响应包 README 已同步移除旧限制。

response 包覆盖率为 72.6%，高于变更前 70%，仍有未达 85% 的既有覆盖率缺口；未扩展其他响应或分页功能，未运行真实存储集成及全量回归。
