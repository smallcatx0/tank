package telemetry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
	"serv_tpl/internal/requestctx"
	"serv_tpl/pkg/kafka"
)

func memoryRuntime(t testing.TB, enabled bool) (*Runtime, *tracetest.InMemoryExporter) {
	t.Helper()
	r, err := New(context.Background(), Options{Metrics: MetricsOptions{Enable: true}})
	require.NoError(t, err)
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { require.NoError(t, tp.Shutdown(context.Background())) })
	r.Provider = tp
	r.Options.Tracing.Enable = enabled
	return r, exp
}

func TestRegistryAndHTTPMetrics(t *testing.T) {
	r, _ := memoryRuntime(t, false)
	other, _ := memoryRuntime(t, false)
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: "private_total", Help: "test"})
	require.NoError(t, r.Registry.Register(c))
	require.Error(t, r.Registry.Register(c))
	require.NoError(t, other.Registry.Register(c))
	r.Metrics.BeginHTTP("GET", "/items/:id")(201, "ok")
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	require.Equal(t, 200, w.Code)
	for _, want := range []string{"go_goroutines", "service_http_inflight 0", `service_http_requests_total{method="GET",outcome="ok",route="/items/:id",status="201"} 1`} {
		require.Contains(t, w.Body.String(), want)
	}
	w = httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	require.Equal(t, 404, w.Code)
	for _, path := range []string{"//metrics", "/metrics/", "/metrics/../healthz"} {
		w = httptest.NewRecorder()
		r.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 404, w.Code)
	}
	off, err := New(context.Background(), Options{})
	require.NoError(t, err)
	require.Nil(t, off.Registry)
	off.Metrics.BeginHTTP("GET", "/")(200, "ok")
	require.NoError(t, off.Shutdown(context.Background()))
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE"} {
		require.Equal(t, method, HTTPMethod(method))
	}
	require.Equal(t, "OTHER", HTTPMethod("secret"))
	require.True(t, ExcludedHTTP("/ready"))
	require.False(t, ExcludedHTTP("/items"))
}

func TestKafkaPropagationAndAttempts(t *testing.T) {
	r, exp := memoryRuntime(t, true)
	parent, end := r.StartProducer(context.Background(), "orders")
	headers := r.InjectKafka(parent, []kafka.Header{{Key: "custom", Value: []byte("keep")}, {Key: "traceparent", Value: []byte("old")}})
	msg := kafka.Message{Topic: "orders", Partition: 1, Offset: 42, Headers: headers}
	var ids []trace.SpanContext
	h := r.KafkaHandler(func(ctx context.Context, _ kafka.Message) error {
		ids = append(ids, trace.SpanContextFromContext(ctx))
		return errors.New("secret")
	})
	require.Error(t, h(context.Background(), msg))
	require.Error(t, h(context.Background(), msg))
	end(nil)
	require.Equal(t, ids[0].TraceID(), ids[1].TraceID())
	require.NotEqual(t, ids[0].SpanID(), ids[1].SpanID())
	spans := exp.GetSpans()
	require.Len(t, spans, 3)
	require.Equal(t, spans[0].Parent, trace.SpanContextFromContext(parent).WithRemote(true))
	require.Equal(t, codes.Error, spans[0].Status.Code)
	// 无消息父链路不能继承 Run 上的 span，取消和 Value 仍保留。
	type key struct{}
	run, cancel := context.WithCancel(context.WithValue(parent, key{}, "value"))
	cancel()
	clean := r.ExtractKafka(run, nil)
	require.False(t, trace.SpanContextFromContext(clean).IsValid())
	require.ErrorIs(t, clean.Err(), context.Canceled)
	require.Equal(t, "value", clean.Value(key{}))
	bad := [][]kafka.Header{{{Key: "traceparent", Value: []byte("bad")}}, {{Key: "traceparent", Value: headers[1].Value}, {Key: "traceparent", Value: []byte("different")}}, {{Key: "tracestate", Value: []byte("bad value")}}, {{Key: "traceparent", Value: []byte(strings.Repeat("a", 4097))}}}
	for _, hs := range bad {
		require.False(t, trace.SpanContextFromContext(r.ExtractKafka(parent, hs)).IsValid())
	}
	require.PanicsWithValue(t, "original", func() {
		_ = r.KafkaHandler(func(context.Context, kafka.Message) error { panic("original") })(context.Background(), msg)
	})
	require.NoError(t, r.KafkaHandler(func(context.Context, kafka.Message) error { return nil })(run, msg))
	spans = exp.GetSpans()
	require.Equal(t, codes.Error, spans[len(spans)-1].Status.Code)
	off, _ := memoryRuntime(t, false)
	require.NoError(t, off.KafkaHandler(func(ctx context.Context, _ kafka.Message) error {
		require.NotEmpty(t, requestctx.TraceID(ctx))
		require.Empty(t, requestctx.SpanID(ctx))
		return nil
	})(context.Background(), msg))
	_, finish := off.StartProducer(context.Background(), "orders")
	finish(nil)
	clone := r.InjectKafka(parent, headers)
	clone[0].Value[0] = 'X'
	require.Equal(t, "keep", string(headers[0].Value))
}

func TestOTLPPayloadAndEnvironment(t *testing.T) {
	received := make(chan *collector.ExportTraceServiceRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, "/v1/traces", req.URL.Path)
		require.Empty(t, req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get("Content-Encoding"))
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var msg collector.ExportTraceServiceRequest
		require.NoError(t, proto.Unmarshal(data, &msg))
		received <- &msg
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	defer server.Close()
	for k, v := range map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://invalid.invalid:1", "OTEL_EXPORTER_OTLP_TRACES_HEADERS": "Authorization=secret", "OTEL_RESOURCE_ATTRIBUTES": "secret=leak,service.name=wrong", "OTEL_SERVICE_NAME": "wrong", "OTEL_TRACES_SAMPLER": "always_off", "OTEL_EXPORTER_OTLP_TRACES_COMPRESSION": "gzip", "OTEL_BSP_MAX_QUEUE_SIZE": "1", "OTEL_SPAN_ATTRIBUTE_COUNT_LIMIT": "1"} {
		t.Setenv(k, v)
	}
	r, err := New(context.Background(), Options{ServiceName: "expected", Tracing: TracingOptions{Enable: true, Endpoint: server.URL + "/v1/traces", SampleRatio: 1, ExportTimeout: time.Second, ShutdownTimeout: time.Second}})
	require.NoError(t, err)
	_, span := r.Tracer().Start(context.Background(), "GET /items/:id")
	span.SetAttributes(attribute.String("url.full", "https://secret/?token=secret"), attribute.String("http.route", "/items/:id"))
	span.RecordError(errors.New("secret"))
	span.SetStatus(codes.Error, "secret")
	span.End()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer upstream.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: r.HTTPTransport(transport), Timeout: time.Second}
	response, e := client.Get(upstream.URL + "/secret?token=secret")
	require.NoError(t, e)
	require.NoError(t, response.Body.Close())
	require.NoError(t, r.Shutdown(context.Background()))
	require.NoError(t, r.Shutdown(context.Background()))
	select {
	case msg := <-received:
		raw := msg.String()
		require.NotContains(t, raw, "secret")
		require.NotContains(t, raw, "wrong")
		require.Contains(t, raw, "expected")
		require.Contains(t, raw, "/items/:id")
	case <-time.After(2 * time.Second):
		t.Fatal("missing OTLP export")
	}
}

func TestHTTPClientPropagation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			r, exp := memoryRuntime(t, enabled)
			ctx := r.Propagator.Extract(context.Background(), propagation.MapCarrier{"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				require.Contains(t, req.Header.Get("traceparent"), "4bf92f3577b34da6a3ce929d0e0e4736")
				require.Empty(t, req.Header.Get("tracestate"))
				w.WriteHeader(204)
			}))
			defer server.Close()
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			client := http.Client{Transport: r.HTTPTransport(transport), Timeout: time.Second}
			req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			require.NoError(t, err)
			req.Header.Set("tracestate", "stale=secret")
			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Empty(t, req.Header.Get("traceparent"))
			if enabled {
				require.Len(t, exp.GetSpans(), 1)
			} else {
				require.Empty(t, exp.GetSpans())
			}
		})
	}
}

func BenchmarkKafkaObservation(b *testing.B) {
	for _, mode := range []string{"off", "unsampled", "sampled"} {
		b.Run(mode, func(b *testing.B) {
			enabled := mode != "off"
			r, _ := memoryRuntime(b, enabled)
			if !enabled {
				r.Metrics = nil
			}
			sampler := sdktrace.NeverSample()
			if mode == "sampled" {
				sampler = sdktrace.AlwaysSample()
			}
			provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithSyncer(tracetest.NewNoopExporter()))
			defer provider.Shutdown(context.Background())
			r.Provider = provider
			h := r.KafkaHandler(func(context.Context, kafka.Message) error { return nil })
			b.ReportAllocs()
			for b.Loop() {
				_ = h(context.Background(), kafka.Message{Topic: "bench"})
			}
		})
	}
}
