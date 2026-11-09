package telemetry

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSamplingParents(t *testing.T) {
	for _, ratio := range []float64{0, 1} {
		for _, parent := range []string{"", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"} {
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))))
			ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent})
			ctx, span := provider.Tracer("test").Start(ctx, "root")
			require.True(t, trace.SpanContextFromContext(ctx).IsValid())
			span.End()
			want := ratio == 1
			if parent != "" {
				want = parent[len(parent)-1] == '1'
			}
			require.Equal(t, want, len(exporter.GetSpans()) == 1)
			require.NoError(t, provider.Shutdown(context.Background()))
		}
	}
}

func TestUnavailableExporterDoesNotBlockBusiness(t *testing.T) {
	r, err := New(context.Background(), Options{ServiceName: "test", Tracing: TracingOptions{Enable: true, Endpoint: "http://127.0.0.1:1/v1/traces", SampleRatio: 1, ExportTimeout: 50 * time.Millisecond, ShutdownTimeout: time.Second}})
	require.NoError(t, err)
	started := time.Now()
	for range 3000 {
		_, span := r.Tracer().Start(context.Background(), "bounded")
		span.End()
	}
	require.Less(t, time.Since(started), time.Second)
	// 队列满只丢 span，导出失败不使业务调用失败。
	_ = r.Shutdown(context.Background())
	require.Less(t, time.Since(started), 2*time.Second)
}

func TestFlushTimeout(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-req.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	r, err := New(context.Background(), Options{ServiceName: "test", Tracing: TracingOptions{Enable: true, Endpoint: server.URL + "/v1/traces", SampleRatio: 1, ExportTimeout: time.Second, ShutdownTimeout: 30 * time.Millisecond}})
	require.NoError(t, err)
	_, span := r.Tracer().Start(context.Background(), "flush")
	span.End()
	start := time.Now()
	err = r.Shutdown(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

type failingExporter struct{}

func (failingExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return errors.New("private endpoint")
}
func (failingExporter) Shutdown(context.Context) error { return errors.New("shutdown") }
func TestExporterFailureRateLimit(t *testing.T) {
	e := &safeExporter{next: failingExporter{}}
	require.NoError(t, e.ExportSpans(context.Background(), nil))
	first := e.last
	require.NoError(t, e.ExportSpans(context.Background(), nil))
	require.Equal(t, first, e.last)
	require.Error(t, e.Shutdown(context.Background()))
}

func TestExporterDoesNotTrustEnvironmentCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("untrusted TLS must fail before HTTP") }))
	defer server.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE", "true")
	r, err := New(context.Background(), Options{ServiceName: "tls", Tracing: TracingOptions{Enable: true, Endpoint: server.URL + "/v1/traces", SampleRatio: 1, ExportTimeout: 100 * time.Millisecond, ShutdownTimeout: time.Second}})
	require.NoError(t, err)
	_, span := r.Tracer().Start(context.Background(), "test")
	span.End()
	_ = r.Shutdown(context.Background())
}
