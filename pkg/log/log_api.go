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
	for one := range extra {
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
	for one := range extra {
		tmpStr, _ := json.Marshal(one)
		extSlice = append(extSlice, string(tmpStr))
	}
	ZapLoger.Debug(msg, zap.String("request_id", requestID), zap.Strings("extra", extSlice))
}

func Warn(msg string, extra ...string) {
	ZapLoger.Warn(msg, zap.Strings("extra", extra))
}

func Error(msg string, extra ...string) {
	ZapLoger.Error(msg, zap.Strings("extra", extra))
}

func DPanic(msg string, extra ...string) {
	ZapLoger.DPanic(msg, zap.Strings("extra", extra))
}

func Panic(msg string, extra ...string) {
	ZapLoger.Panic(msg, zap.Strings("extra", extra))
}

func Fatal(msg string, extra ...string) {
	ZapLoger.Fatal(msg, zap.Strings("extra", extra))
}
