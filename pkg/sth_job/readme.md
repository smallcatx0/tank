# pkg/sth_job 使用文档（sthjob 任务队列框架）

## 项目简介

轻量级通用任务调度框架，支持「Redis 消息队列消费」与「数据库表任务 + 分布式锁」双模式，统一封装任务消费、状态管理、监控采集与优雅关闭的全生命周期。

- **RmqJob**：高实时消息队列消费组件，适合核心实时链路；
- **DbJob**：可持久化的数据库任务调度组件，适合兜底补偿、离线加工等需要审计/重跑的场景。

## 文件结构

| 文件 | 说明 |
| --- | --- |
| `db_job.go` | `DbJob` 组件、`ITask` 接口、任务状态定义、`Hostname()` |
| `rmq_job.go` | `RmqJob` 组件（基于 [adjust/rmq/v5](https://github.com/adjust/rmq) 二次封装） |

包名：`package sthjob`，导入路径 `gtank/pkg/sth_job`（项目内习惯以 `sthjob "gtank/pkg/sth_job"` 方式导入）。

## 选型建议

| 场景 | 推荐 | 原因 |
| --- | --- | --- |
| 高实时消费、多消费者并行限速 | `RmqJob` | Redis List 队列，毫秒级投递 |
| 任务需持久化、可审计、可手工干预重跑 | `DbJob` | 任务即表记录，状态全程落库 |
| 不想引入额外 MQ 中间件 | `DbJob` | 仅依赖 MySQL + Redis |

---

## 一、DbJob —— 数据库表任务调度

### 1. 工作原理

以「数据库表即队列 + Redis 分布式锁」实现多实例分布式消费：

```
┌─────────────┐   SetNX 取锁    ┌──────────────────────────────┐
│ dbComsumer  │ ──────────────> │ GetTasks 批量拉取 status=1    │
│ (取任务协程) │                 │ UpdateStatus -> 10 (执行中)   │
└─────────────┘                 └──────────────┬───────────────┘
      释放锁后立即放行其他实例消费                │ 放入 taskBuff 缓冲通道
                                                ▼
┌──────────────┐  消费缓冲    ┌─────────────────────────────────┐
│ WorkerNum 个 │ ───────────> │ t.Run(db) 执行业务，返回状态      │
│ goroutine    │              └──────────────┬──────────────────┘
└──────────────┘                             ▼
                                 TaskBatDone 开启时：成功 id 写入 taskDoneBuff，
                                 后台协程每秒聚合、200 条一批落库（status=30）
```

- **短锁设计**：只在「取任务 + 置执行中状态」期间持有 Redis 锁（key 为 `bs:job:{表名}:{jobName}`，SetNX TTL 300s），拿到任务改完状态立即释放，支撑多实例并行消费；
- **状态流转**：`1 init（初始）→ 10 runing（执行中）→ 30 done（完成）`；失败状态（如 20）由消费者在 `Run()` 中自行写库维护（含错误次数），框架不干预；
- **超时重置兜底**：可选开启，定时把超过 `TaskTimeout` 仍处于「执行中」的任务重置为初始状态，自动恢复卡死任务；
- **优雅关闭**：`Close()` 停止取任务，将缓冲区中未执行的任务回写为 init、已完成未落库的 id 回写为 done，最大限度避免任务丢失。

### 2. 接入步骤：实现 ITask 接口

业务任务表模型只需实现以下 6 个方法：

```go
type ITask interface {
    ID() int64                                        // 任务主键
    TableName() string                                // 任务表名（也用于拼锁 key）
    GetTasks(*gorm.DB, int) ([]ITask, error)          // 按 limit 拉取一批待执行任务（通常查 status=1）
    UpdateStatus(*gorm.DB, []int64, TaskStatus) error // 按 id 批量更新状态
    StatusReset(*gorm.DB, time.Duration) (int64, error) // 将超时的"执行中"任务重置为初始，返回影响行数
    Run(*gorm.DB) TaskStatus                          // 业务执行逻辑，返回最终状态
}
```

实现示例参考 `internal/task/sth_task.go` 中的 `BsSthTask`（可用 `var _ sthjob.ITask = &BsSthTask{}` 在编译期校验接口实现）。

> 注意：任务表需要有 `status` 与 `updated_at` 字段配合状态流转和超时重置；`GetTasks` 的查询条件（如 `ORDER BY id`、失败重试条件）由业务自行决定，框架不限制。

### 3. 创建与启动

```go
func StartSthTask() func() {
    job, err := sthjob.NewDbJob(dao.MysqlCli, dao.RedisCli, &BsSthTask{}, "try_sth_task")
    if err != nil {
        return nil
    }
    job.Logger = glog.L() // 默认 zap.L()，可替换

    // 调优参数（不设置则用包内默认值）
    job.SetBufferNum(10)          // 任务缓冲区大小，默认 20
    job.SetDoneBuff(2000)         // 开启"成功状态批量回写"，并指定 done 缓冲区大小
    job.WorkerNum = 2             // 消费协程数，默认 4
    job.BatNum = 10               // 每次从库里拉取的任务条数，默认 10
    job.NoTaskSleep = 60          // 无任务时休眠秒数，默认 300
    job.NeedTimeoutReset = true   // 开启超时重置，默认 false
    job.TimeoutResetD = 60        // 超时检查间隔秒数
    job.TaskTimeout = 300         // "执行中"超过该秒数视为超时

    job.Start()                   // 启动全部后台协程（非阻塞）
    return func() { job.Close() } // 返回优雅关闭函数，交给退出流程调用
}
```

### 4. 配置项速查

| 字段/方法 | 默认值 | 说明 |
| --- | --- | --- |
| `WorkerNum` | 4 | 消费协程数量 |
| `BatNum` | 10 | 单批量拉取任务数 |
| `NoTaskSleep` | 300（秒） | 无任务休眠时长 |
| `NoLockedSleep` | 10（秒） | 未抢到锁休眠时长 |
| `SetBufferNum(n)` | 20 | `taskBuff` 缓冲区容量 |
| `SetDoneBuff(n)` | 不开启 | 调用后启用成功状态批量回写（每秒聚合、200 条一批） |
| `NeedTimeoutReset` / `TimeoutResetD` / `TaskTimeout` | false | 超时重置开关、检查间隔（秒）、任务超时（秒） |

## 二、RmqJob —— Redis 消息队列消费

### 1. 工作原理

基于 `adjust/rmq` 封装队列创建、多 Worker 注册、限速消费：

- `StartConsuming(rate, time.Second)`：每秒从 Redis 拉取 `rate` 条到本地内存分发给各 Consumer，天然限流；
- 每个实例自动注册多个消费者（tag 为 `主机名_队列名#序号`）；
- 后台定时（默认 1 分钟）用 `rmq.Cleaner` 清理未 ACK 的任务，防止任务卡死丢失；
- `Close()` 等待本地在途消息消费完毕后再停止，实现优雅关闭。

### 2. 接入步骤

**第一步：实现 `rmq.Consumer`**（该库要求的接口，签名为 `Consume(dv rmq.Delivery)`）：

```go
func (q SthRetryWorker) Consume(dv rmq.Delivery) {
    task := &SthMqTask{}
    if err := task.Unserialize(dv.Payload()); err != nil {
        _ = dv.Ack() // 反序列化失败重试也无意义，记日志后直接 ACK
        return
    }
    task.Run()
    if task.Status == RmqStatus_fail {
        _ = q.failQ.PublishBytes(task.Serialize()) // 可重试错误丢入重试队列
    }
    _ = dv.Ack() // 务必在处理完成后 ACK
}
```

**第二步：创建队列并注册消费者**：

```go
func StartRmqTask() func() {
    mainQ, err := sthjob.NewRmqJob(dao.RedisCli, "sth_task_main")
    if err != nil { return nil }
    retryQ, err := sthjob.NewRmqJob(dao.RedisCli, "sth_task_fail")
    if err != nil { return nil }

    workers := []rmq.Consumer{
        SthRetryWorker{failQ: retryQ.Queue()},
        SthRetryWorker{failQ: retryQ.Queue()},
    }
    if err := mainQ.Start(workers, 10); err != nil { return nil } // 限速 10 条/秒
    if err := retryQ.Start(workers, 4); err != nil { return nil }

    go mainQ.MeticLog(time.Second * 10) // 每 10s 输出一次队列指标
    return func() {
        mainQ.Close()
        retryQ.Close()
    }
}
```

**第三步：生产端投递消息**（通过 `Queue()` 拿到原生 `rmq.Queue`）：

```go
mainQ.Queue().PublishBytes(task.Serialize())
```

### 3. API 一览

| 方法 | 说明 |
| --- | --- |
| `NewRmqJob(cli *redis.Client, name string) (*RmqJob, error)` | 创建连接并打开队列 |
| `Start(workers []rmq.Consumer, rate int64) error` | 开始限速消费，注册全部 Worker |
| `Queue() rmq.Queue` | 获取原生队列实例（Publish / 二级投递用） |
| `Metic() (rmq.Stats, error)` | 采集一次队列指标 |
| `MeticLog(dt time.Duration)` | 周期性输出指标日志（需自行 `go` 调用） |
| `Close()` | 阻塞直到消费优雅停止 |
| `UnAckedCleanD` 字段 | 未 ACK 清理间隔，默认 `time.Minute` |

> 注意：`MeticLog` 是**阻塞死循环**，务必 `go q.MeticLog(...)` 启动；`ClearUnAcked` 协程随 `Start` 自动启动且无法停止，进程退出随进程结束即可。

## 三、优雅关闭与进程入口

`StartSthTask` / `StartRmqTask` 均约定**返回一个 `func()` 关闭函数**，由 worker 入口统一在收到退出信号后调用：

```go
rmqClose := task.StartRmqTask()
bootstrap.WaitingExit(rmqClose) // 收到 SIGINT/SIGTERM 后执行关闭函数
```

参考 `cmd/sth_worker/main.go`（当前只启用了一种任务，另一种以注释保留，可共存）。

## 四、注意事项与已知限制

- **DbJob 未调用 `SetDoneBuff` 时**：`TaskBatDone` 为 false，`Run()` 返回 `TaskStatus_done` 后不会自动置 done，需要业务在 `Run()` 里自行更新状态；同理，失败状态完全由业务在 `Run()` 中写入；
- **DbJob 抢锁失败的行为**：`dbComsumer` 未抢到锁时按 `NoLockedSleep` 配置休眠（默认 10 秒）后进入下一轮重试；
- **RmqJob 消费者必须显式 ACK**：未 ACK 的消息会被定时清理，但延迟体现在清理周期（默认 1 分钟）之后；
- 锁 key、缓冲区等默认常量定义在 `db_job.go` 顶部（`LogKeyTpl`、`BatNum`、`WorkerNum`、`JobBufferCount` 等），全局调整可直接修改；
- `Hostname()` 结果进程内缓存，获取失败时为 `"unknow"`，用于锁 value 与消费者 tag，多实例部署时保证容器 hostname 唯一更利于排查。
