// Package logger 基于 zap 的日志工厂：提供标准输出/按天切割文件两种日志介质，
// 并通过 zap 官方全局实例（zap.L / zap.ReplaceGlobals）承载进程级默认 logger。
package logger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 日志等级配置值（与配置文件 log.level 的取值一致，供调用方对照使用）
const (
	Level_Debug = "debug"
	Level_Info  = "info"
	Level_Warn  = "warn"
	Level_Error = "error"
	Level_Panic = "panic"
	Level_Fatal = "fatal"
)

func init() {
	// 经 SetGlobal 承载默认实例：全局持有者自包加载起即登记，
	// L() 恒非 nil，且 SetGlobalLevel 在启动装配前亦可用（作用于兑底实例）。
	SetGlobal(&Holder{zap: zap.NewExample(), level: zap.NewAtomicLevelAt(zapcore.InfoLevel)})
}

// toLevel 将配置值映射为 zapcore 日志等级，未知取值回退 info。
func toLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case Level_Debug:
		return zapcore.DebugLevel
	case Level_Info:
		return zapcore.InfoLevel
	case Level_Warn:
		return zapcore.WarnLevel
	case Level_Error:
		return zapcore.ErrorLevel
	case Level_Panic:
		return zapcore.PanicLevel
	case Level_Fatal:
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Holder 将日志实例与其构造时的等级句柄绑成一个对象：
// 句柄字段非导出，对外只暴露能力（SetLevel）不见配件；
// zap.AtomicLevel 内部为指针语义，经值拷贝传入 core 后 SetLevel 仍写回同一实例。
type Holder struct {
	zap   *zap.Logger
	level zap.AtomicLevel
	close func() error
}

// Logger 返回底层 zap 实例（方案 A 决策：zap 即 API，调用方拿到的就是 *zap.Logger）。
func (h *Holder) Logger() *zap.Logger { return h.zap }

// SetLevel 调整该实例等级，经实例挂载的 core 即时生效，并发安全。
func (h *Holder) SetLevel(level string) { h.level.SetLevel(toLevel(level)) }

// Close 刷新并关闭底层 writer（stdout 实例为 nil writer，为空操作）。
func (h *Holder) Close() error {
	if h.close == nil {
		return nil
	}
	return errors.Join(h.zap.Sync(), h.close())
}

// 当前全局实例对应的持有者，供 SetGlobalLevel 动态调级。
var globalHolder atomic.Pointer[Holder]

// SetGlobalLevel 动态调整全局默认日志实例的等级，立即作用于其 core。
func SetGlobalLevel(level string) {
	if h := globalHolder.Load(); h != nil {
		h.SetLevel(level)
	}
}

// jsonEncoder 统一 JSON 编码格式。
func jsonEncoder() zapcore.Encoder {
	config := zap.NewProductionEncoderConfig()
	// 覆盖默认配置
	config.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	return zapcore.NewJSONEncoder(config)
}

// fileWriterByDay 按天切割日志文件：在文件名主干与扩展名之间注入日期占位符。
func fileWriterByDay(filename string) (zapcore.WriteSyncer, func() error, error) {
	ext := filepath.Ext(filename)
	path := filepath.Dir(filename)
	file := strings.TrimSuffix(filepath.Base(filename), ext)
	pattern := filepath.Join(path, file+"-%Y-%m-%d"+ext)

	hook, err := rotatelogs.New(
		pattern,
		rotatelogs.WithMaxAge(time.Hour*24*365),
		rotatelogs.WithRotationTime(time.Hour*24),
	)
	if err != nil {
		return nil, nil, err
	}
	return zapcore.AddSync(hook), hook.Close, nil
}

// NewFileLogger 新建按天切割的文件日志实例。
// close 字段用于刷新并关闭底层切割 writer。
func NewFileLogger(filename, level string) (h *Holder, err error) {
	al := zap.NewAtomicLevelAt(toLevel(level))
	encoder := jsonEncoder()
	writer, close, err := fileWriterByDay(filename)
	if err != nil {
		return nil, err
	}
	core := zapcore.NewCore(encoder, writer, al)
	return &Holder{zap: zap.New(core, zap.AddCaller()), level: al, close: close}, nil
}

// NewStdLogger 新建控制台（stdout）日志实例。
// stdout 生命周期与进程一致，无需关闭，故 close 恒为 nil。
func NewStdLogger(level string) (h *Holder, err error) {
	al := zap.NewAtomicLevelAt(toLevel(level))
	encoder := jsonEncoder()
	core := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), al)
	return &Holder{zap: zap.New(core, zap.AddCaller()), level: al}, nil
}

// SetGlobal 装载进程级默认日志实例，并登记其持有者供 SetGlobalLevel 调级；
// 同步刷新被替换的旧实例。并发安全由 zap 官方实现保证（内部以原子指针承载全局实例）。
func SetGlobal(h *Holder) {
	old := zap.L()
	globalHolder.Store(h)
	zap.ReplaceGlobals(h.Logger())
	_ = old.Sync()
}

// L 返回包内登记的全局日志实例（与 SetGlobal 装载的持有者同源，
// 保证 L()、SetGlobalLevel、调级句柄指向同一实例）；若持有者被直接
// 清空则回退 zap.L() 兑底，任何路径不返回 nil。
func L() *zap.Logger {
	if h := globalHolder.Load(); h != nil {
		return h.Logger()
	}
	return zap.L()
}

// SysStatInfo 采集并记录机器指标。
// 约定：mem_percent / cpu_percent 均为 0~100 的百分比原值（浮点），
// goroutine 为当前 goroutine 数。
func SysStatInfo(l *zap.Logger) { SysStatInfoContext(context.Background(), l) }

// SysStatInfoContext 支持取消采样，便于退出时等待采样真正结束。
func SysStatInfoContext(ctx context.Context, l *zap.Logger) {
	info := []zap.Field{
		zap.String("type", "sysmetrics"),
	}
	hostname, err := os.Hostname()
	if err != nil {
		l.Error("[sys_stat] get hostname fail, " + err.Error())
	} else {
		info = append(info, zap.String("hostname", hostname))
	}
	memInfo, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		l.Error("[sys_stat] get memory info fail, " + err.Error())
	} else {
		info = append(info, zap.Float64("mem_percent", memInfo.UsedPercent))
	}
	// cpu_percent 返回 0~100 的百分比，与 mem_percent 单位对齐
	cpuInfo, err := cpu.PercentWithContext(ctx, time.Second*5, false)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		l.Error("[sys_stat] get cpu info fail, " + err.Error())
	} else if len(cpuInfo) > 0 {
		sum := float64(0)
		for _, v := range cpuInfo {
			sum += v
		}
		info = append(info, zap.Float64("cpu_percent", sum/float64(len(cpuInfo))))
	}
	info = append(info, zap.Int("goroutine", runtime.NumGoroutine()))
	l.Info("system stat metrics", info...)
}
