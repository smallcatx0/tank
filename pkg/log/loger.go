// Package glog is a global logger
// logger: this is extend package, use https://github.com/uber-go/zap
package glog

import (
	"os"
	"strings"
	"tank/pkg/helper"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	//Logger  *zap.Logger 日志记录器
	Logger *zap.SugaredLogger
	//AtomLevel 日志最小级别
	AtomLevel = zap.NewAtomicLevel()
)

// C 日志配置
type C struct {
	Driver     string
	Path       string
	LogLevel   string
	MaxSize    int
	MaxAge     int
	MaxBackups int
}

// InitLog 初始化日志文件
func InitLog(c *C) {
	SetLevel(c.LogLevel)
	var core zapcore.Core
	// 打印至文件中
	if c.Driver == "file" {
		configs := zap.NewProductionEncoderConfig()
		configs.EncodeTime = zapcore.ISO8601TimeEncoder
		w := zapcore.AddSync(&lumberjack.Logger{
			Filename:   helper.GetDefStr(c.Path, "./logs/current.log"), // 日志文件的位置
			MaxSize:    helper.GetDefInt(c.MaxSize, 32),                // MB
			LocalTime:  true,                                           // 是否使用自己本地时间
			Compress:   true,                                           // 是否压缩/归档旧文件
			MaxAge:     helper.GetDefInt(c.MaxAge, 30),                 // 保留旧文件的最大天数
			MaxBackups: helper.GetDefInt(c.MaxBackups, 300),            // 保留旧文件的最大个数
		})

		core = zapcore.NewCore(
			zapcore.NewJSONEncoder(configs),
			w,
			AtomLevel,
		)
	} else {
		// 打印在控制台
		consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		core = zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stdout), AtomLevel.Level())
	}

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Logger = logger.Sugar()
}

// SetLevel 动态设置日志级别 level=[debug,info,warn,error]
func SetLevel(level string) {
	loglevel := zapcore.InfoLevel
	switch strings.ToLower(level) {
	case "debug":
		loglevel = zapcore.DebugLevel
	case "info":
		loglevel = zapcore.InfoLevel
	case "warn":
		loglevel = zapcore.WarnLevel
	case "error":
		loglevel = zapcore.ErrorLevel
	default:
		loglevel = zapcore.InfoLevel
	}
	AtomLevel.SetLevel(loglevel)
}

func Debug(args ...interface{}) {
	Logger.Debug(args...)
}

func Debugf(template string, args ...interface{}) {
	Logger.Debugf(template, args...)
}

func Info(args ...interface{}) {
	Logger.Info(args...)
}

func Infof(template string, args ...interface{}) {
	Logger.Infof(template, args...)
}

func Warn(args ...interface{}) {
	Logger.Warn(args...)
}

func Warnf(template string, args ...interface{}) {
	Logger.Warnf(template, args...)
}

func Error(args ...interface{}) {
	Logger.Error(args...)
}

func Errorf(template string, args ...interface{}) {
	Logger.Errorf(template, args...)
}
