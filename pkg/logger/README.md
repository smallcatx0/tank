# pkg/logger 使用文档

基于 [zap](https://go.uber.org/zap) 的日志工厂包：提供**控制台**与**按天切割文件**两种日志介质、进程级全局默认实例，以及**运行时动态调整日志等级**的能力。

## 设计决策

- **zap 即 API**：本包不屏蔽 zap，调用方拿到的就是 `*zap.Logger`，所有 zap 方法（`Info/Error/With/Named...`）直接使用；包的价值在**构造工厂、全局实例承载与等级句柄绑定**，而非再造一层接口。
- **Holder 绑定实例与等级句柄**：`zap` 不支持从已构造实例取回等级写入口，故构造期将 `AtomicLevel` 与实例绑进 `Holder`；句柄字段非导出，对外只暴露能力（`SetLevel`），不见配件。
- **配置语义不出装配层**：`"file"`/`"std"` 等配置取值与 `log.*` 键的映射属于 `internal/bootstrap`，本包只提供工厂函数，保持 `pkg/` 无业务依赖。

## 快速开始

### 独立使用

```go
package main

import (
    "go.uber.org/zap"

    "serv_tpl/pkg/logger"
)

func main() {
    // 1. 创建实例（等级取值见下表，未知取值回退 info）
    h, err := logger.NewStdLogger("info")
    if err != nil {
        panic(err)
    }
    defer h.Close()

    // 2. 装载为进程级默认实例（同时登记等级句柄）
    logger.SetGlobal(h)

    // 3. 任何位置通过 L() 使用（zap 即 API，字段用 zap.Field 构造器）
    logger.L().Info("服务启动", zap.String("key", "value"))
}
```

### 项目内装配使用（推荐路径）

常规业务代码**不要**自行构造日志实例，启动装配已由 `internal/bootstrap.MustInitLog()` 完成，直接使用全局实例即可：

```go
import "serv_tpl/pkg/logger"

logger.L().Error("处理失败: " + err.Error())
```

配置文件（`conf/app.yaml`）相关键：

```yaml
log:
  type: file          # file | 其他值走 stdout；语义解析在 bootstrap 完成
  path: logs/app.log  # 仅 file 必填，不兼容 log.filename
  level: info         # debug | info | warn | error | panic | fatal
metrics:
  enable: true       # 开启可取消的系统采样心跳
  dt: 10             # 秒
```

## API 参考

### 工厂

| 函数 | 说明 |
| --- | --- |
| `NewFileLogger(filename, level string) (*Holder, error)` | 按天切割的文件日志。切割规则：在文件名主干与扩展名之间注入日期，如 `logs/app.log` → `app-2026-10-22.log`，保留 365 天 |
| `NewStdLogger(level string) (*Holder, error)` | 控制台日志（stdout，JSON 格式） |

### Holder

| 方法 | 说明 |
| --- | --- |
| `Logger() *zap.Logger` | 取出底层 zap 实例（zap 即 API） |
| `SetLevel(level string)` | 调整**该实例**等级，经其 core 即时生效，并发安全 |
| `Close() error` | Sync 并关闭底层切割 writer；stdout 实例为空操作 |

### 全局实例

| 函数 | 说明 |
| --- | --- |
| `SetGlobal(h *Holder)` | 装载进程级默认实例并登记等级句柄，同步刷新旧实例（旧实例会被 Sync） |
| `L() *zap.Logger` | 返回包内登记的全局实例，与 `SetGlobalLevel` 作用对象同源，恒非 nil |
| `SetGlobalLevel(level string)` | 动态调整**全局实例**等级，立即生效；不串扰未登记的独立实例 |
| `SysStatInfo(l *zap.Logger)` | 采集并记录机器指标（见下文字段约定） |

等级取值 ↔ 常量对照（`Level_*` 定义于本包）：

| 配置值 | 常量 | zap 等级 |
| --- | --- | --- |
| `debug` | `Level_Debug` | DebugLevel |
| `info` | `Level_Info` | InfoLevel |
| `warn` | `Level_Warn` | WarnLevel |
| `error` | `Level_Error` | ErrorLevel |
| `panic` | `Level_Panic` | PanicLevel |
| `fatal` | `Level_Fatal` | FatalLevel |
| 其他/空 | — | 回退 InfoLevel |

## 动态调级

```go
// 场景：线上排障临时放开 debug，无需重启
logger.SetGlobalLevel("debug")
// ... 排障结束恢复
logger.SetGlobalLevel("info")
```

- 全局调级：`SetGlobalLevel` 作用于 `SetGlobal` 装载的实例（`L()` 同源）。
- 实例调级：对 `Holder` 调用 `h.SetLevel`，多实例可各自独立调整；只有经 `SetGlobal` 登记的实例才受 `SetGlobalLevel` 影响。
- 包加载时 `init()` 已登记兜底实例，`L()` 与 `SetGlobalLevel` 在启动装配完成前亦可安全调用。

## 生命周期

- **文件实例**：writer 持有文件句柄，退出前必须 `Close()`（`bootstrap.MustInitLog` 返回的清理闭包已封装）。
- **stdout 实例**：无需关闭，`Close()` 为空操作。
- 全局实例替换（`SetGlobal`）时旧实例会被 Sync，未关闭的旧文件 writer 需调用方自行管理——装配流程中通常不存在此场景。

## SysStatInfo 字段约定

心跳指标日志（`l.Info("system stat metrics", ...)`）的结构化字段，**单位约定为契约，下游解析依赖，不得随意更改**：

| 字段 | 类型 | 单位/含义 |
| --- | --- | --- |
| `type` | string | 固定 `"sysmetrics"` |
| `hostname` | string | 主机名（采集失败时缺失并输出 Error 日志） |
| `mem_percent` | float64 | 内存使用率，0~100 百分比原值 |
| `cpu_percent` | float64 | CPU 使用率，0~100 百分比原值（5 秒采样窗口，多核均值） |
| `goroutine` | int | 当前 goroutine 数 |

注意：`cpu.Percent` 采样会阻塞 5 秒，只应在心跳/定时任务中调用，不要置于请求路径。

## 并发安全

- `L()`、`SetGlobal`、`SetGlobalLevel`、`Holder.SetLevel` 均可并发调用：全局槽由 `atomic.Pointer[Holder]` 与 zap 官方原子机制承载，等级句柄 `zap.AtomicLevel` 本身并发安全。
- 测试含三路并发竞态用例（读 `L()` / 写 `SetGlobal` / 调 `SetGlobalLevel`），验证命令：

```bash
go test -race ./pkg/logger/...
```

## 启动与退出接入

HTTP 入口：`go run ./cmd/api -config conf/app.yaml`。启动仍要求有效 MySQL/Redis 配置。启动装配首先登记日志清理，运行编排逆序关闭资源，使日志最后关闭。关闭错误影响进程退出码。

心跳使用 `SysStatInfoContext` 取消采样并等待退出；`SysStatInfo` 保留兼容入口。HTTP 排空、存储回收和日志关闭共用总退出期限，二次信号与总超时强退不等待日志 Sync。
