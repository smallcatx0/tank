package telemetry

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"serv_tpl/internal/requestctx"
	"serv_tpl/pkg/kafka"
	"strings"
)

// ExtractKafka 忽略运行上下文的 span，只从消息提取；保留期限和业务值。
func (r *Runtime) ExtractKafka(ctx context.Context, headers []kafka.Header) context.Context {
	clean := trace.ContextWithSpanContext(ctx, trace.SpanContext{})
	carrier := propagation.MapCarrier{}
	size := 0
	for _, h := range headers {
		key := strings.ToLower(h.Key)
		if key != "traceparent" && key != "tracestate" {
			continue
		}
		size += len(h.Key) + len(h.Value)
		if size > 4096 {
			return clean
		}
		value := string(h.Value)
		if old, ok := carrier[key]; ok && old != value {
			return clean
		}
		carrier[key] = value
	}
	if state := carrier.Get("tracestate"); state != "" {
		if _, err := trace.ParseTraceState(state); err != nil {
			return clean
		}
	}
	return r.Propagator.Extract(clean, carrier)
}

// InjectKafka 返回独立 header 副本，只替换 W3C 字段，不修改消息视图。
func (r *Runtime) InjectKafka(ctx context.Context, headers []kafka.Header) []kafka.Header {
	carrier := propagation.MapCarrier{}
	r.Propagator.Inject(ctx, carrier)
	out := make([]kafka.Header, 0, len(headers)+2)
	for _, h := range headers {
		key := strings.ToLower(h.Key)
		if key == "traceparent" || key == "tracestate" {
			continue
		}
		out = append(out, kafka.Header{Key: h.Key, Value: append([]byte(nil), h.Value...)})
	}
	for _, key := range []string{"traceparent", "tracestate"} {
		if v := carrier[key]; v != "" {
			out = append(out, kafka.Header{Key: key, Value: []byte(v)})
		}
	}
	return out
}

// KafkaHandler 每次尝试创建独立消费 span，不改变错误、panic 或提交语义。
func (r *Runtime) KafkaHandler(next kafka.Handler) kafka.Handler {
	return func(ctx context.Context, m kafka.Message) (err error) {
		ctx = r.ExtractKafka(ctx, m.Headers)
		ctx = requestctx.WithTrace(ctx, "")
		if !r.Options.Tracing.Enable {
			return next(ctx, m)
		}
		ctx, span := r.Tracer().Start(ctx, m.Topic+" process", trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attribute.String("messaging.system", "kafka"), attribute.String("messaging.destination.name", m.Topic), attribute.Int64("messaging.kafka.partition", int64(m.Partition)), attribute.Int64("messaging.kafka.offset", m.Offset)))
		defer func() {
			if p := recover(); p != nil {
				span.SetStatus(codes.Error, "")
				span.End()
				panic(p)
			}
			if err != nil || ctx.Err() != nil {
				span.SetStatus(codes.Error, "")
			}
			span.End()
		}()
		return next(ctx, m)
	}
}

// StartProducer 为一次发送创建 span；调用者在 Broker 确认或失败后调用结束函数。
func (r *Runtime) StartProducer(ctx context.Context, topic string) (context.Context, func(error)) {
	if !r.Options.Tracing.Enable {
		return ctx, func(error) {}
	}
	child, span := r.Tracer().Start(ctx, topic+" publish", trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("messaging.system", "kafka"), attribute.String("messaging.destination.name", topic)))
	return child, func(err error) {
		if err != nil || child.Err() != nil {
			span.SetStatus(codes.Error, "")
		}
		span.End()
	}
}
