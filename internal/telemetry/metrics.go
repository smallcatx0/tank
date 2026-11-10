package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
	"time"
)

// Metrics 固定指标集合，标签必须先归一化后调用。
type Metrics struct {
	httpCount, grpcCount       *prometheus.CounterVec
	httpDuration, grpcDuration *prometheus.HistogramVec
	httpInflight, grpcInflight prometheus.Gauge
}

func newMetrics(reg *prometheus.Registry) *Metrics {
	buckets := []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	m := &Metrics{
		httpCount:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "service_http_requests_total", Help: "Completed HTTP requests."}, []string{"method", "route", "status", "outcome"}),
		grpcCount:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "service_grpc_requests_total", Help: "Completed registered RPCs."}, []string{"service", "method", "code"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "service_http_request_duration_seconds", Help: "HTTP duration.", Buckets: buckets}, []string{"method", "route"}),
		grpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "service_grpc_request_duration_seconds", Help: "RPC duration.", Buckets: buckets}, []string{"service", "method"}),
		httpInflight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_http_inflight", Help: "Active HTTP requests."}),
		grpcInflight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "service_grpc_inflight", Help: "Active registered RPCs."}),
	}
	reg.MustRegister(m.httpCount, m.grpcCount, m.httpDuration, m.grpcDuration, m.httpInflight, m.grpcInflight)
	return m
}

// BeginHTTP 开始统计，返回只调用一次的完成回调；禁用时无副作用。
func (m *Metrics) BeginHTTP(method, route string) func(int, string) {
	if m == nil {
		return func(int, string) {}
	}
	start := time.Now()
	m.httpInflight.Inc()
	return func(status int, outcome string) {
		m.httpInflight.Dec()
		m.httpCount.WithLabelValues(method, route, strconv.Itoa(status), outcome).Inc()
		m.httpDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

// HTTPMethod 将任意请求方法收敛为有限标签。
func HTTPMethod(method string) string {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE":
		return method
	}
	return "OTHER"
}

// ExcludedHTTP 排除健康及抓取流量。
func ExcludedHTTP(path string) bool {
	return path == "/healthz" || path == "/ready" || path == "/metrics"
}
