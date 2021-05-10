package glog

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

func Debug(msg string, extra ...string) {
	var request_id string
	if len(extra) >= 1 {
		request_id = extra[0]
		extra = extra[1:]
	}
	ZapLoger.Debug(msg, zap.String("request_id", request_id), zap.Strings("extra", extra))
}

func DebugF(requestID, template string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	ZapLoger.Debug(msg, zap.String("request_id", requestID))
}

func DebugT(requestID, msg string, extra ...interface{}) {
	extSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.Debug(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func Info(msg string, extra ...string) {
	var request_id string
	if len(extra) >= 1 {
		request_id = extra[0]
		extra = extra[1:]
	}
	ZapLoger.Info(msg, zap.String("request_id", request_id), zap.Strings("extra", extra))
}

func InfoF(requestID, template string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	ZapLoger.Info(msg, zap.String("request_id", requestID))
}

func InfoT(requestID, msg string, extra ...interface{}) {
	extSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.Info(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func Warn(msg string, extra ...string) {
	var request_id string
	if len(extra) >= 1 {
		request_id = extra[0]
		extra = extra[1:]
	}
	ZapLoger.Warn(msg, zap.String("request_id", request_id), zap.Strings("extra", extra))
}

func WarnF(requestID, template string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	ZapLoger.Warn(msg, zap.String("request_id", requestID))
}

func WarnT(requestID, msg string, extra ...interface{}) {
	extSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.Warn(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func Error(msg string, extra ...string) {
	var request_id string
	if len(extra) >= 1 {
		request_id = extra[0]
		extra = extra[1:]
	}
	ZapLoger.Error(msg, zap.String("request_id", request_id), zap.Strings("extra", extra))
}

func ErrorF(requestID, template string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	ZapLoger.Error(msg, zap.String("request_id", requestID))
}

func ErrorT(requestID, msg string, extra ...interface{}) {
	extSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.Error(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func DPanic(msg string, extra ...string) {
	var request_id string
	if len(extra) >= 1 {
		request_id = extra[0]
		extra = extra[1:]
	}
	ZapLoger.DPanic(msg, zap.String("request_id", request_id), zap.Strings("extra", extra))
}

func DPanicF(requestID, template string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	ZapLoger.DPanic(msg, zap.String("request_id", requestID))
}

func DPanicT(requestID, msg string, extra ...interface{}) {
	extSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.DPanic(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func Panic(msg string, extra ...string) {
	var request_id string
	if len(extra) >= 1 {
		request_id = extra[0]
		extra = extra[1:]
	}
	ZapLoger.Panic(msg, zap.String("request_id", request_id), zap.Strings("extra", extra))
}

func PanicF(requestID, template string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	ZapLoger.Panic(msg, zap.String("request_id", requestID))
}

func PanicT(requestID, msg string, extra ...interface{}) {
	extSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.Panic(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func Fatal(msg string, extra ...string) {
	var request_id string
	if len(extra) >= 1 {
		request_id = extra[0]
		extra = extra[1:]
	}
	ZapLoger.Fatal(msg, zap.String("request_id", request_id), zap.Strings("extra", extra))
}

func FatalF(requestID, template string, args ...interface{}) {
	msg := fmt.Sprintf(template, args...)
	ZapLoger.Fatal(msg, zap.String("request_id", requestID))
}

func FatalT(requestID, msg string, extra ...interface{}) {
	extSlice := make([]string, 0, len(extra))
	for _, one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.Fatal(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func Sync() {
	ZapLoger.Sync()
}
