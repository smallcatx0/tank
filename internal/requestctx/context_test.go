package requestctx

import (
	"context"
	"github.com/stretchr/testify/require"
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
