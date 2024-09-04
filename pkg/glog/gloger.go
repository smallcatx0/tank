package glog

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// 日志写入类型
	Medium_Std  = "std"
	Medium_File = "file"

	// 日志等级
	Level_Debug = "debug"
	Level_Info  = "info"
	Level_Warn  = "warn"
	Level_Error = "error"
	Level_Panic = "panic"
)

type GLog struct {
	Level     zap.AtomicLevel
	LevelName string
	Medium    string // 日志写入介质
	FileName  string // 文件名
	zap       *zap.Logger
}

var (
	// 默认实列
	defIns = &GLog{
		Medium: Medium_Std,
		Level:  zap.NewAtomicLevel(),
		zap:    zap.NewExample(),
	}
)

func D() *GLog {
	return defIns
}

// 新建文件实例
func NewFileLogger(filename, level string) (*GLog, error) {
	l := GLog{
		FileName:  filename,
		LevelName: level,
		Medium:    Medium_File,
		Level:     zap.NewAtomicLevel(),
	}
	// 动态设置日志级别
	l.SetLevel(level)

	// json格式
	config := zap.NewProductionEncoderConfig()
	// 覆盖默认配置
	config.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	encoder := zapcore.NewJSONEncoder(config)

	// 按天切割日志写入文件
	writer, err := fileWriterByDay(filename)
	if err != nil {
		return nil, err
	}

	core := zapcore.NewCore(encoder, writer, l.Level)
	l.zap = zap.New(
		core,
		zap.AddCaller(),
	)
	return &l, nil
}

// 新建控制台实例
func NewStdLogger(level string) (*GLog, error) {
	l := GLog{
		LevelName: level,
		Medium:    Medium_Std,
		Level:     zap.NewAtomicLevel(),
	}
	// 动态设置日志级别
	l.SetLevel(level)
	// json格式
	encoderConf := zap.NewProductionEncoderConfig()
	encoderConf.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewConsoleEncoder(encoderConf)
	core := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), l.Level)
	l.zap = zap.New(
		core,
		zap.AddCaller(),
	)
	return &l, nil
}
func (l *GLog) Z() *zap.Logger {
	return l.zap
}
func (l *GLog) SetLevel(level string) {
	loglevel := zapcore.InfoLevel
	switch strings.ToLower(level) {
	case Level_Debug:
		loglevel = zapcore.DebugLevel
	case Level_Info:
		loglevel = zapcore.InfoLevel
	case Level_Warn:
		loglevel = zapcore.WarnLevel
	case Level_Panic:
		loglevel = zapcore.ErrorLevel
	default:
		loglevel = zapcore.InfoLevel
	}
	l.Level.SetLevel(loglevel)
}

func fileWriterByDay(filename string) (zapcore.WriteSyncer, error) {
	ext := filepath.Ext(filename)
	path := filepath.Dir(filename)
	file := filepath.Base(filename)
	filebase := file[:len(file)-len(ext)]
	filename = filebase + "-%Y-%m-%d" + ext
	filename = filepath.Join(path, filename)

	hook, err := rotatelogs.New(
		filename,
		rotatelogs.WithMaxAge(time.Hour*24*365),
		rotatelogs.WithRotationTime(time.Hour*24),
	)

	if err != nil {
		return nil, err
	}
	return zapcore.AddSync(hook), nil
}
