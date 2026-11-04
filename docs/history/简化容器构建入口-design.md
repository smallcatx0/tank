<!-- D21 | 状态：已完成 -->
# 简化容器构建入口

## 问题

用户要求移除 Dockerfile 的 SERVICE 判断，后续 worker 通过新增独立 Dockerfile 构建。计划入口见 [plan.md](../../plan.md)。

## 规则与约束

遵循 [AGENTS.md](../../AGENTS.md) 和 [项目规范](../../项目规范.md)。

## 方案与决策记录

- 根 Dockerfile 固定构建 ./cmd/api，移除 SERVICE 参数和 case 判断。
- 保留 Alpine、shell、时区、非 root 用户及配置挂载方式。
- README 删除 SERVICE 和现有 worker 镜像构建/运行命令，说明后续按需新增独立 Dockerfile；本次不新增 worker Dockerfile。
- 验收：API 镜像构建成功，程序 -help 可执行，文档无失效的 SERVICE 用法。

## 状态勾点（关键里程碑）

- [x] 用户明确方案。
- [x] 修改构建与说明。
- [x] 构建及入口验证。
- [x] 用户验收并归档。

## 验收结论

2026-11-04：API 镜像 gtank-d21:api 构建成功，无网络环境执行 -help 以 0 退出；git diff --check 通过，Dockerfile 和 README 无 SERVICE 参数或旧 worker 镜像用法。本次未修改 Go 代码，未重复 Go 全量回归。用户确认验收通过，完成收尾归档，无遗留事项。
