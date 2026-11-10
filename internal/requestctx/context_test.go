package requestctx

import (
	"context"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestMetadata(t *testing.T) {
	require.Empty(t, TraceID(context.Background()))
	require.Zero(t, LatencyMS(context.Background()))
	parent, cancel := context.WithCancel(context.Background())
	ctx := WithTrace(parent, "abc")
	require.Equal(t, "abc", TraceID(ctx))
	require.GreaterOrEqual(t, LatencyMS(ctx), float64(0))
	cancel()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestSpanContextPriority(t *testing.T) {
	tid, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	sid, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid})
	ctx := trace.ContextWithSpanContext(WithTrace(context.Background(), "legacy"), sc)
	require.Equal(t, tid.String(), TraceID(ctx))
	require.Equal(t, sid.String(), SpanID(ctx))
	require.Empty(t, SpanID(trace.ContextWithRemoteSpanContext(ctx, sc)))
	require.Empty(t, SpanID(context.Background()))
}
