package httpmd

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"serv_tpl/internal/telemetry"
)

const panicKey = "serv_tpl.http.panic"

// Observe 在恢复中间件外统计真实结果，并同步响应与日志的关联标识。
func Observe(r *telemetry.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		if telemetry.ExcludedHTTP(c.Request.URL.Path) {
			Trace(c)
			return
		}
		ctx := r.Propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := telemetry.HTTPMethod(c.Request.Method)
		var span trace.Span
		if r.Options.Tracing.Enable {
			ctx, span = r.Tracer().Start(ctx, method+" "+route, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()
		}
		c.Request = c.Request.WithContext(ctx)
		// Trace 同步 Gin key 和响应头，并在内部运行日志、恢复及业务处理。
		done := r.Metrics.BeginHTTP(method, route)
		Trace(c)
		outcome := httpOutcome(c)
		done(c.Writer.Status(), outcome)
		if span != nil {
			span.SetAttributes(attribute.String("http.request.method", method), attribute.String("http.route", route), attribute.Int("http.response.status_code", c.Writer.Status()))
			if outcome != "ok" && outcome != "client_error" {
				span.SetStatus(codes.Error, "")
			}
		}
	}
}

func httpOutcome(c *gin.Context) string {
	if c.GetBool(panicKey) {
		return "panic"
	}
	if errors.Is(c.Request.Context().Err(), context.Canceled) {
		return "canceled"
	}
	if errors.Is(c.Request.Context().Err(), context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if c.Writer.Status() >= 500 {
		return "server_error"
	}
	if c.Writer.Status() >= 400 {
		return "client_error"
	}
	return "ok"
}
