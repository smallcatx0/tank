package telemetry

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"serv_tpl/pkg/logger"
	"sync"
	"time"
)

// safeSpan 是只读出口视图，不改写 SDK 中原始 span。
type safeSpan struct {
	sdktrace.ReadOnlySpan
	resource *resource.Resource
}

func (s safeSpan) Resource() *resource.Resource { return s.resource }

func (s safeSpan) Attributes() []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(s.ReadOnlySpan.Attributes()))
	for _, a := range s.ReadOnlySpan.Attributes() {
		switch string(a.Key) {
		case "http.request.method", "http.method", "http.route", "http.response.status_code", "http.status_code", "rpc.system", "rpc.service", "rpc.method", "rpc.grpc.status_code", "messaging.system", "messaging.destination.name", "messaging.kafka.partition", "messaging.kafka.offset":
			out = append(out, a)
		}
	}
	return out
}
func (s safeSpan) Status() sdktrace.Status {
	v := s.ReadOnlySpan.Status()
	v.Description = ""
	return v
}
func (s safeSpan) Events() []sdktrace.Event { return nil }
func (s safeSpan) Links() []sdktrace.Link   { return nil }

type safeExporter struct {
	next     sdktrace.SpanExporter
	resource *resource.Resource
	mu       sync.Mutex
	last     time.Time
}

func (e *safeExporter) report(err error) {
	if err == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if time.Since(e.last) >= time.Minute {
		e.last = time.Now()
		logger.L().Warn("追踪导出失败", zap.String("component", "telemetry"))
	}
}
func (e *safeExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	views := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, s := range spans {
		views[i] = safeSpan{ReadOnlySpan: s, resource: e.resource}
	}
	err := e.next.ExportSpans(ctx, views)
	e.report(err)
	// 错误已限频记录，不把可能含端点信息的错误交给全局 SDK 诊断输出。
	return nil
}
func (e *safeExporter) Shutdown(ctx context.Context) error {
	err := e.next.Shutdown(ctx)
	e.report(err)
	return err
}
