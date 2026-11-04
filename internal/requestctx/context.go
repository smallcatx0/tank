// Package requestctx 承载与传输协议无关的请求关联信息。
package requestctx

import (
	"context"
	"time"
)

type key struct{}
type metadata struct {
	id      string
	started time.Time
}

// WithTrace 写入请求标识及开始时间，供业务与日志关联请求。
func WithTrace(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, key{}, metadata{id: id, started: time.Now()})
}

// TraceID 读取请求标识；非请求任务返回空字符串。
func TraceID(ctx context.Context) string {
	m, _ := ctx.Value(key{}).(metadata)
	return m.id
}

// LatencyMS 返回请求已耗时，非请求任务返回零。
func LatencyMS(ctx context.Context) float64 {
	m, ok := ctx.Value(key{}).(metadata)
	if !ok {
		return 0
	}
	return float64(time.Since(m.started)) / float64(time.Millisecond)
}
