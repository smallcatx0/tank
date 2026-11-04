package dao

import (
	"context"
	"go.uber.org/zap"
	gormlog "gorm.io/gorm/logger"
	"serv_tpl/internal/errinfo"
	"serv_tpl/internal/requestctx"
	"serv_tpl/pkg/logger"
	"time"
)

// sqlLogger 不格式化 SQL、参数或驱动消息，仅记录安全的执行诊断字段。
type sqlLogger struct{ level gormlog.LogLevel }

func (l sqlLogger) LogMode(level gormlog.LogLevel) gormlog.Interface { l.level = level; return l }

// Info 忽略 ORM 的自由文本消息，诊断统一由 Trace 输出。
func (l sqlLogger) Info(context.Context, string, ...interface{}) {}

// Warn 忽略 ORM 的自由文本消息，避免意外包含参数。
func (l sqlLogger) Warn(context.Context, string, ...interface{}) {}

// Error 忽略 ORM 的自由文本消息，数据库错误由 Trace 分类记录。
func (l sqlLogger) Error(context.Context, string, ...interface{}) {}

// Trace 不调用 SQL 格式化回调，即使 Debug 开启也不泄露查询原文。
func (l sqlLogger) Trace(ctx context.Context, begin time.Time, _ func() (string, int64), err error) {
	if l.level == gormlog.Silent {
		return
	}
	elapsed := time.Since(begin)
	fields := []zap.Field{zap.String("operation", "database.query"), zap.String("trace_id", requestctx.TraceID(ctx)),
		zap.Float64("latency_ms", float64(elapsed)/float64(time.Millisecond))}
	if err != nil && l.level >= gormlog.Error {
		logger.L().Error("数据库操作失败", append(fields, errinfo.Fields(err)...)...)
	} else if elapsed >= 200*time.Millisecond && l.level >= gormlog.Warn {
		logger.L().Warn("数据库慢操作", fields...)
	} else if l.level >= gormlog.Info {
		logger.L().Info("数据库操作", fields...)
	}
}
