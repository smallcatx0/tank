<!-- D36 | 状态：已完成（上游同步，本仓库重编号续接） -->
# 可观测性部署验证

## 问题
验证真实 Prometheus 抓取及 Collector 到追踪后端的完整链路，补充代码测试之外的部署证据。

## 规则与约束
遵守 [工程规范](../../项目规范.md)、[本地隔离约定](../../AGENTS.md) 与 [观测契约](../../internal/telemetry/README.md)。实际机器信息仅保存于受忽略的本地目录。

## 方案与决策记录
用户同意临时 Collector + Jaeger。复用现有 Prometheus，备份并临时添加隔离验证目标，完成后恢复配置。使用仓库真实 bootstrap、HTTP 中间件与 gRPC instrumentation 构造 HTTP 客户端 → HTTP 服务 → gRPC 客户端 → gRPC 服务链路，不访问共享业务数据库。验证 target up、Go/进程指标、HTTP/gRPC 计数增长及后端同 TraceID 四个 span 的父子关系。临时容器、进程及配置完成后清理；证据保留本地。不提交。

## 状态勾点（关键里程碑）
- [x] 环境盘点及临时后端方案确认
- [x] Prometheus 抓取与计数验证
- [x] 完整链路查询验证
- [x] 恢复环境及验收记录

## 验收结论
2026-11-10：通过。真实 Prometheus 两个隔离目标均 up=1，Go/进程指标可查询；3 次请求使 HTTP/gRPC Counter 各增长 3。Collector 转发到 Jaeger 后，按 TraceID 及服务名均查到完整四段 HTTP → gRPC 链路；逐项验证唯一根、client/server 顺序、父子 span ID 及日志关联。

使用固定 Collector 0.123.0 与 Jaeger all-in-one 1.76.0（内存存储）；较新 Collector 镜像下载停滞后改用上述版本。验证程序复用实际观测组件，但不执行 API 存储初始化、不覆盖 Kafka 消息链路、持久追踪存储及生产规模性能。后端不可达时请求仍成功，就绪后新请求可完整导出。

验证进程正常退出，临时容器及网络已清理；原 Prometheus 配置通过哈希一致性、promtool 和目标查询核对，原服务保持运行。机器地址、TraceID、原始查询响应及日志仅保存在受忽略的本地验证目录。生产代码未修改，本轮不提交。
