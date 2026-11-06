package task

import (
	"context"
	"github.com/stretchr/testify/require"
	"serv_tpl/pkg/kafka"
	"testing"
)

func TestLogKafkaMessage(t *testing.T) {
	require.NoError(t, LogKafkaMessage(context.Background(), kafka.Message{Topic: "example", Offset: 42}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, LogKafkaMessage(ctx, kafka.Message{}), context.Canceled)
}
