package httpmd

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"net/http/httptest"
	"serv_tpl/internal/requestctx"
	"serv_tpl/internal/telemetry"
	"testing"
	"time"
)

func TestObservationContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, enabled := range []bool{false, true} {
		for _, metrics := range []bool{false, true} {
			r, err := telemetry.New(context.Background(), telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: metrics}})
			require.NoError(t, err)
			r.Options.Tracing.Enable = enabled
			exp := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
			r.Provider = provider
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
			engine := gin.New()
			engine.Use(Observe(r), ReqLog, Recovery)
			engine.GET("/items/:id", func(c *gin.Context) {
				require.Equal(t, c.GetString(RequestIDKey), requestctx.TraceID(c.Request.Context()))
				if enabled {
					require.NotEmpty(t, requestctx.SpanID(c.Request.Context()))
				} else {
					require.Empty(t, requestctx.SpanID(c.Request.Context()))
				}
				c.Status(201)
			})
			engine.GET("/panic", func(c *gin.Context) { c.String(200, "written"); panic("private") })
			engine.GET("/error", func(c *gin.Context) { c.Status(503) })
			engine.GET("/healthz", func(c *gin.Context) { c.Status(200) })
			for _, path := range []string{"/items/123", "/panic", "/error", "/missing/123", "/healthz"} {
				req := httptest.NewRequest("GET", path, nil)
				req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
				req.Header.Set(RequestIDHeader, "abcdefabcdefabcd")
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				if path != "/healthz" {
					require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", w.Header().Get(RequestIDHeader))
				}
			}
			if enabled {
				require.Len(t, exp.GetSpans(), 4)
			} else {
				require.Empty(t, exp.GetSpans())
			}
			if metrics {
				w := httptest.NewRecorder()
				r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
				require.Contains(t, w.Body.String(), `outcome="panic",route="/panic",status="200"`)
				require.Contains(t, w.Body.String(), `route="unmatched"`)
				require.NotContains(t, w.Body.String(), "/missing/123")
				require.Contains(t, w.Body.String(), "service_http_inflight 0")
			}
		}
	}
}

func TestHTTPOutcome(t *testing.T) {
	for _, tc := range []struct {
		status   int
		cancel   bool
		deadline bool
		want     string
	}{{200, true, false, "canceled"}, {200, false, true, "deadline_exceeded"}, {400, false, false, "client_error"}, {500, false, false, "server_error"}, {200, false, false, "ok"}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx := context.Background()
		if tc.cancel {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		if tc.deadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer cancel()
		}
		c.Request = httptest.NewRequest("GET", "/", nil).WithContext(ctx)
		c.Status(tc.status)
		require.Equal(t, tc.want, httpOutcome(c))
	}
}

func BenchmarkHTTPObservation(b *testing.B) {
	for _, mode := range []string{"off", "unsampled", "sampled"} {
		b.Run(mode, func(b *testing.B) {
			enabled := mode != "off"
			gin.SetMode(gin.TestMode)
			r, _ := telemetry.New(context.Background(), telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: enabled}})
			r.Options.Tracing.Enable = enabled
			sampler := sdktrace.NeverSample()
			if mode == "sampled" {
				sampler = sdktrace.AlwaysSample()
			}
			provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithSyncer(tracetest.NewNoopExporter()))
			defer provider.Shutdown(context.Background())
			r.Provider = provider
			e := gin.New()
			e.Use(Observe(r), Recovery)
			e.GET("/bench", func(c *gin.Context) { c.Status(204) })
			b.ReportAllocs()
			for b.Loop() {
				e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/bench", nil))
			}
		})
	}
}
