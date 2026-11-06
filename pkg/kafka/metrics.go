package kafka

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	events            *prometheus.CounterVec
	duration          *prometheus.HistogramVec
	inflight, pending prometheus.Gauge
}

func newMetrics(o Options) metrics {
	labels := prometheus.Labels{"consumer": o.ClientID, "group": o.GroupID}
	return metrics{
		events:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "kafka_consumer_events_total", Help: "消费处理、重试、关闭失败与再均衡次数。", ConstLabels: labels}, []string{"event"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "kafka_consumer_duration_seconds", Help: "业务处理与提交耗时。", ConstLabels: labels, Buckets: prometheus.DefBuckets}, []string{"stage"}),
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "kafka_consumer_inflight", Help: "已登记的在途消息数。", ConstLabels: labels}),
		pending:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "kafka_consumer_pending", Help: "本轮尚未开始处理的消息数。", ConstLabels: labels}),
	}
}

// Describe 实现 prometheus.Collector；调用方显式向自己的注册器注册消费者。
func (c *Consumer) Describe(ch chan<- *prometheus.Desc) {
	c.metrics.events.Describe(ch)
	c.metrics.duration.Describe(ch)
	c.metrics.inflight.Describe(ch)
	c.metrics.pending.Describe(ch)
}

// Collect 收集消费者指标；构造函数不会注册默认全局注册器。
func (c *Consumer) Collect(ch chan<- prometheus.Metric) {
	c.metrics.events.Collect(ch)
	c.metrics.duration.Collect(ch)
	c.metrics.inflight.Collect(ch)
	c.metrics.pending.Collect(ch)
}
