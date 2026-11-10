package telemetry

import (
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"net/http"
)

type propagationTransport struct {
	runtime *Runtime
	base    http.RoundTripper
}

func (t propagationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	for _, k := range t.runtime.Propagator.Fields() {
		copy.Header.Del(k)
	}
	t.runtime.Propagator.Inject(copy.Context(), propagation.HeaderCarrier(copy.Header))
	return t.base.RoundTrip(copy)
}

// HTTPTransport 装配出站追踪；调用方保留连接池、超时和响应体关闭所有权。
func (r *Runtime) HTTPTransport(base http.RoundTripper) http.RoundTripper {
	if !r.Options.Tracing.Enable {
		return propagationTransport{r, base}
	}
	return propagationTransport{r, otelhttp.NewTransport(base, otelhttp.WithTracerProvider(r.Provider), otelhttp.WithPropagators(r.Propagator), otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()))}
}
