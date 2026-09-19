# gtank 架构说明

> 一个结构规范、分层清晰、可快速落地的 Go HTTP 服务端脚手架，内置统一响应/异常体系、链路追踪、分布式任务消费、配置热加载与优雅退出等生产级能力。

---

**不断迭代，永远要与自己和解**

## 总体概览

### 多进程模型

同一套代码产出多个可执行体，共享 `bootstrap / models / pkg` 等基础设施：

| 进程        | 入口                       | 职责                                                  |
| --------- | ------------------------ | --------------------------------------------------- |
| API 服务    | `cmd/main.go`            | 对外提供 HTTP 接口（用户端 `/v1`、后台 `/admin`、探活）              |
| 任务 Worker | `cmd/sth_worker/main.go` | 常驻消费异步任务（消息队列 / 数据库表任务） ；作为独立的 worker 端，可**快速水平扩容** |

### 技术栈

| 领域      | 选型                          | 说明                        |
| ------- | --------------------------- | ------------------------- |
| Web 框架  | Gin v1.10.0                 | 路由、中间件、参数绑定               |
| ORM     | GORM v1.21.9 + mysql driver | 数据访问，SQL 日志接管至 zap        |
| 缓存 / 队列 | go-redis/v9                 | 缓存、分布式锁、轻量消息队列            |
| 消息队列    | adjust/rmq/v5               | 基于 Redis 的高性能可靠队列         |
| 定时任务    | robfig/cron/v3              | 秒级 cron，配合分布式锁防重          |
| 鉴权      | golang-jwt/v4               | JWT 签发与解析                 |
| 配置      | viper v1.7.1                | YAML 配置、支持热加载             |
| 日志      | zap v1.27.0 + rotatelogs    | 结构化 JSON 日志、按时间切割         |
| 系统监控    | gopsutil/v3                 | CPU / 内存 / goroutine 指标采集 |
| 导出      | excelize v2 + gjson         | JSON 流式写入 Excel、分页导出      |
| 日志投递    | aliyun-log-go-sdk           | 阿里云 SLS 生产者               |
| 工具      | lancet / uuid / testify     | 通用工具、唯一 ID、测试断言           |

---

## 分层架构

### 分层视图

```
                        ┌──────────────────────────────────────────┐
   进程入口              │  cmd/main.go (API)   cmd/sth_worker (任务)  │
                        └───────────────┬────────────────────────────┘
                                        │ 统一编排
                        ┌───────────────▼────────────────────────────┐
   引导层 bootstrap     │  配置/日志/DB 初始化 · 心跳监控 · 优雅退出      │
                        └───────────────┬────────────────────────────┘
                                        │
   ┌────────────────────────────────────┼────────────────────────────────────┐
   │                          HTTP 请求链路 (API 进程)                         │
   ▼                                    ▼                                      ▼
┌─────────┐  路由   ┌────────────┐ 校验/绑定 ┌───────────┐  调用  ┌──────────┐ 落库 ┌──────────┐
│ routes  │ ──────> │ controller │ ───────> │models/valid│ ─────>│ service  │ ───> │models/dao│
│ +中间件 │         │  接口入口   │          │  入参规范   │       │ 业务编排  │     │ MySQL/Redis
└─────────┘         └─────┬──────┘          └───────────┘       └──────────┘     └────┬─────┘
                          │ 统一出口                                                   │
                    ┌─────▼──────┐                                              存储/外部接口
                    │ middleware │  响应格式 / 错误码 / 分页 / JWT / traceid
                    │   resp     │
                    └────────────┘

   任务消费链路 (Worker 进程)： pkg/sth_job (RmqJob / DbJob) + pkg/db_strategy (定时策略) ──> models/dao
   横切复用层 pkg： glog · excel · helper · sth_job · db_strategy （已解耦，可跨项目引入）
```

### 依赖方向（单向，禁止反向依赖）

```
cmd → bootstrap → routes → controller → { models/valid, models/service } → models/dao → 存储
                          ↘ middleware ↙            ↘ pkg ↙
```

**约束**：`bootstrap/global` 与 `pkg/*` 作为可复用层，不应引用业务本地包，保持无环与可移植性。

---

## 目录结构与职责

| 目录                   | 分层  | 职责                                                                                                     |
| -------------------- | --- | ------------------------------------------------------------------------------------------------------ |
| `cmd/`               | 入口  | `main.go` API 服务入口；`sth_worker/main.go` 异步任务 Worker 入口                                                 |
| `bootstrap/`         | 引导  | 生命周期编排：`app.go`(App 封装 gin/http.Server、flag)、`init.go`(Conf/Log/DB 初始化、心跳、优雅退出)、`global/`(全局常量，不引用本地包) |
| `conf/`              | 配置  | `app.yaml`(+example) 运行配置，`worker.yaml` 任务进程配置                                                         |
| `internal/conf/`     | 配置  | 基于 viper 的配置单例 `AppConf` 与访问器（Env/Debug/Port）                                                          |
| `routes/`            | 路由  | 路由总入口，注册健康检查、`/v1` 对外接口、`/admin` 后台接口                                                                  |
| `controller/`        | 接口  | HTTP 逻辑入口：参数接收、调用服务、返回响应（`health.go`、`v1/`）                                                            |
| `middleware/httpmd/` | 中间件 | `SetHeader`(traceid)、`ReqLog`(请求日志)、`JwtAuth`(鉴权)                                                      |
| `middleware/resp/`   | 中间件 | **统一响应格式、异常体系、错误码、分页**（详见该包 readme）                                                                    |
| `models/valid/`      | 校验  | 入参模型定义 + 绑定校验 + JWT 生成/解析（详见该包 readme）                                                                 |
| `models/service/`    | 业务  | 业务编排（如 `mq.go` Redis 列表队列的 HTTP 推送生产/消费）                                                               |
| `models/dao/`        | 数据  | 数据访问：`mysql.go`/`redis.go` 连接初始化；子包 `rdb`(Redis 工具/队列)、`rds`(用户模型)、`cal`(验证码)、`slscal`(SLS 投递)         |
| `pkg/`               | 复用  | 已解耦可复用组件：`sth_job`、`db_strategy`、`glog`、`excel`、`helper`                                               |
| `doc/`               | 文档  | 规范、接口文档、分布式任务消费、定时任务平台等设计文档                                                                            |

---

## 核心流程

### 服务启动（API 进程）

```
init: InitFlag 解析命令行(-config/-help)
main:
  1. InitConf   加载 YAML 配置（viper，含默认值兜底）
  2. InitLog    按配置初始化 zap（stdout / 按时间切割的文件日志）
  3. InitDB     初始化 MySQL(gorm) 与 Redis，配置连接池
  4. Heartbeat  开启系统指标周期采集（CPU/内存/goroutine）
  5. NewApp     构建 gin 引擎（debug/release 模式）
  6. Use        挂载全局中间件：SetHeader(traceid) → ReqLog
  7. RegisterRoutes  注册全部路由
  8. Run        启动 HTTP 监听（非阻塞 goroutine）
  9. WaitExit   阻塞等待信号，触发优雅关闭
```

### 一次鉴权请求的生命周期

```
Client ──HTTP──> /v1/user/info
  → SetHeader    生成/透传 x-b3-traceid，写入 Context 与响应头
  → JwtAuth      解析 Authorization token，成功则 c.Set("jwtinfo", 用户信息)
  → controller   valid.UserInfo(c) 命中上下文缓存；valid.Bind*AndCheck 绑定并校验入参
  → service/dao  查询 MySQL / Redis
  → resp.Succ/Fail 统一封装 {errcode, msg, data, request_id} 输出
```

### 优雅退出

`bootstrap.WaitingExit` 捕获 `SIGINT/SIGTERM/SIGHUP`，**逆序执行注册的关闭函数**：HTTP `Shutdown`(5s 超时) → 任务 Worker 的 `job.Close()`（将缓冲区未完成任务回滚为初始、已完成回写 done），最大限度避免请求/任务中断与丢失。

---

## 核心模块详解

### bootstrap —— 引导与生命周期

- `app.go`：`App` 结构封装 `gin.Engine` 与 `http.Server`，提供 `NewApp/RegisterRoutes/Run/Stop/WaitExit`；并集中命令行参数解析（`-config`、`-help`）。
- `init.go`：`InitConf`/`InitLog`/`InitDB` 分步初始化，`Heartbeat` 周期采集系统指标，`WaitingExit` 统一信号处理与优雅关闭。

### middleware/resp —— 统一响应与异常

- 所有出口固定 `{errcode, msg, data, request_id}` 结构；HTTP 状态码与业务码绑定。
- `Exception` 类型 + 预置错误码（参数错误 10000、未登录 10004、token 非法 10006 等），普通 error 在生产环境自动隐藏细节、dev 环境回显。
- `Pagination` 与 gorm 集成，一步完成 `Count + Offset/Limit`。
- 详见 `middleware/resp/readme.md`。

### models/valid —— 入参校验与身份

- `binding` tag 基础校验 + `Valid()` 自定义校验两段式，校验错误直接复用 resp 异常体系。
- 承载 JWT 生成/解析与 `UserInfo(c)` 当前登录用户获取（缓存 key 统一为 `jwtinfo`）。
- 详见 `models/valid/readme.md`。

### pkg/sth_job —— 双模式分布式任务框架

- **RmqJob**：基于 adjust/rmq 封装，队列创建、多 Worker 注册、限速消费、未 ACK 定时清理、指标采集、优雅关闭。
- **DbJob**：以「数据库表即队列 + Redis 分布式锁」实现多实例消费，仅依赖 MySQL+Redis；任务状态全程落库可审计可重跑，批量拉取 + 成功状态批量回写降低写压力，短锁设计 + 超时重置兜底。
- 详见 `pkg/sth_job/readme.md`。

### pkg/db_strategy —— 配置驱动的定时数据策略

- 读取数据库配置表（`tabledata_ttl` / `tabledata_retry`）注册 cron 定时任务；
- 到期数据自动分批删除（TTL）、超时状态自动重试更新；Redis 锁保障多实例不重复执行。

### pkg/glog · excel · helper —— 通用复用组件

- `glog`：zap 封装，支持 stdout/文件、按时间切割、动态级别、系统指标日志、钉钉机器人告警；
- `excel`：JSON 流式写入 Excel，支持大数据量分页导出；
- `helper`：`Empty`/`GetDef`/`TouchDir` 等通用工具。

---

## 启动与部署

### 准备配置

```bash
cp conf/app.yaml.example conf/app.yaml         # API 服务配置
# 编辑 mysql / redis / log / http_port 等
```

关键项：`env`(dev/test/pre/prod)、`debug`、`metrics` + `metrics_dt`、`http_port`、`mysql.*`(DSN/连接池)、`redis.*`、`log.type`(stdout/file)。

### 构建与运行

```bash
go build -o bin/api    ./cmd                       # API 服务
go build -o bin/worker ./cmd/sth_worker            # 异步任务 Worker

./bin/api    -config conf/app.yaml                 # 默认读取 conf/app.yaml
./bin/worker -config conf/worker.yaml
```

### 探活接口

| 路径             | 说明           |
| -------------- | ------------ |
| `GET /`        | 版本/描述信息      |
| `GET /healthz` | 存活探针         |
| `GET /ready`   | 就绪探针         |
| `GET /reload`  | 热重载配置、日志与 DB |

---



## TODO

- [ ]  **`pkg/excel` 引入了本地 `pkg/helper`**，`pkg` 作为可复用层宜保持零本地依赖，可将通用函数内联或下沉
