package telemetry

import (
	"context"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"net/http"
	"testing"
	"time"
)

type shutdownExporter struct{ done chan struct{} }

func (*shutdownExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }
func (e *shutdownExporter) Shutdown(context.Context) error                           { close(e.done); return nil }

func TestCanceledShutdownStillStopsBatchProcessor(t *testing.T) {
	exporter := &shutdownExporter{done: make(chan struct{})}
	processor := sdktrace.NewBatchSpanProcessor(exporter)
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(processor))
	r := &Runtime{provider: provider, processor: processor, transport: &http.Transport{}, Options: Options{Tracing: TracingOptions{ShutdownTimeout: time.Second}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, r.Shutdown(ctx), context.Canceled)
	select {
	case <-exporter.done:
	case <-time.After(time.Second):
		t.Fatal("取消的关闭 context 导致批处理 goroutine 未停止")
	}
	require.ErrorIs(t, r.Shutdown(context.Background()), context.Canceled)
}
