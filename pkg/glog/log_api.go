package glog

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"
)

func str2fields(extra []string) []zap.Field {
	fields := make([]zap.Field, 0, 2)
	if len(extra) >= 1 {
		fields = append(fields, zap.String("request_id", extra[0]))
		fields = append(fields, zap.Strings("extra", extra[1:]))
	} else {
		fields = append(fields, zap.Strings("extra", extra))
	}
	return fields
}

func interface2fields(requestID string, extra []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, 2)
	fields = append(fields, zap.String("request_id", requestID))
	extraSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		s, _ := json.Marshal(one)
		extraSlice = append(extraSlice, string(s))
	}
	return append(fields, zap.Strings("extra", extraSlice))
}

func Debug(msg string, extra ...string) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Debug(msg, str2fields(extra)...)
}

func DebugF(template, requestID string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Debug(msg, zap.String("request_id", requestID))
}

func DebugT(msg, requestID string, extra ...interface{}) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Debug(msg, interface2fields(requestID, extra)...)
}

func Info(msg string, extra ...string) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Info(msg, str2fields(extra)...)
}

func InfoF(template, requestID string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Info(msg, zap.String("request_id", requestID))
}

func InfoT(msg, requestID string, extra ...interface{}) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Info(msg, interface2fields(requestID, extra)...)
}

func Warn(msg string, extra ...string) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Warn(msg, str2fields(extra)...)
}

func WarnF(template, requestID string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Warn(msg, zap.String("request_id", requestID))
}

func WarnT(msg, requestID string, extra ...interface{}) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Warn(msg, interface2fields(requestID, extra)...)
}

func Error(msg string, extra ...string) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Error(msg, str2fields(extra)...)
}

func ErrorF(template, requestID string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Error(msg, zap.String("request_id", requestID))
}

func ErrorT(msg, requestID string, extra ...interface{}) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Error(msg, interface2fields(requestID, extra)...)
}

func DPanic(msg string, extra ...string) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).DPanic(msg, str2fields(extra)...)
}

func DPanicF(template, requestID string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).DPanic(msg, zap.String("request_id", requestID))
}

func DPanicT(msg, requestID string, extra ...interface{}) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).DPanic(msg, interface2fields(requestID, extra)...)
}

func Panic(msg string, extra ...string) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Panic(msg, str2fields(extra)...)
}

func PanicF(template, requestID string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Panic(msg, zap.String("request_id", requestID))
}

func PanicT(msg, requestID string, extra ...interface{}) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Panic(msg, interface2fields(requestID, extra)...)
}

func Fatal(msg string, extra ...string) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Fatal(msg, str2fields(extra)...)
}

func FatalF(template, requestID string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Fatal(msg, zap.String("request_id", requestID))
}

func FatalT(msg, requestID string, extra ...interface{}) {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Fatal(msg, interface2fields(requestID, extra)...)
}

func Sync() {
	defIns.Z().WithOptions(zap.AddCallerSkip(1)).Sync()
}

// 机器信息日志记录
func SysStatInfo() {
	info := []zap.Field{
		zap.String("type", "sysmetrics"),
	}
	hostname, err := os.Hostname()
	if err != err {
		info = append(info, zap.String("hostname", hostname))
		Error("[sys_stat] get hostname fail, " + err.Error())
	} else {
		info = append(info, zap.String("hostname", hostname))
	}
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		Error("[sys_stat]get memory info fail, " + err.Error())
	} else {
		info = append(info, zap.Int("mem_percent", int(memInfo.UsedPercent*1000)))
	}
	// cpu信息
	cpuInfo, err := cpu.Percent(time.Second*5, false)
	if err != nil {
		Error("[sys_stat]get cpu info fail, " + err.Error())
	} else if len(cpuInfo) > 0 {
		sum := float64(0)
		for _, v := range cpuInfo {
			sum += v
		}
		info = append(info, zap.Int("cpu_percent", int(sum*1000)/len(cpuInfo)))
	}
	info = append(info, zap.Int("goroutine", runtime.NumGoroutine()))
	D().Z().Info("system stat metrics", info...)
}
