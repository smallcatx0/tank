package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"serv_tpl/internal/global"
	"serv_tpl/pkg/kafka"

	"github.com/stretchr/testify/require"
)

type fakeKafka struct {
	run  func(context.Context, kafka.Handler) error
	stop func(context.Context) error
}

func (f fakeKafka) Run(ctx context.Context, h kafka.Handler) error { return f.run(ctx, h) }
func (f fakeKafka) Shutdown(ctx context.Context) error             { return f.stop(ctx) }
func TestKafkaDisabledAndInvalid(t *testing.T) {
	s := MustKafkaWorker(global.KafkaConfig{}, nil, closedRuntime(t))
	require.Nil(t, s.Done())
	require.NoError(t, s.Shutdown(context.Background()))
	require.Panics(t, func() { MustKafkaWorker(global.KafkaConfig{Enabled: true}, nil, closedRuntime(t)) })
	require.Panics(t, func() {
		MustKafkaWorker(global.KafkaConfig{Enabled: true}, func(context.Context, kafka.Message) error { return nil }, closedRuntime(t))
	})
}
func TestKafkaRunFailure(t *testing.T) {
	for _, mode := range []string{"error", "nil", "panic"} {
		t.Run(mode, func(t *testing.T) {
			failure := errors.New("consume failed")
			s := startKafka(fakeKafka{
				run: func(context.Context, kafka.Handler) error {
					if mode == "panic" {
						panic("secret")
					}
					if mode == "nil" {
						return nil
					}
					return failure
				},
				stop: func(context.Context) error { return nil },
			}, nil, time.Second)
			var order []string
			code := WaitingWorkerExit(2*time.Second, s.Done(), nil,
				Cleanup{Name: "kafka", Stop: func(ctx context.Context) error { order = append(order, "kafka"); return s.Shutdown(ctx) }},
				Cleanup{Name: "log", Stop: func(context.Context) error { order = append(order, "log"); return nil }})
			require.Equal(t, 1, code)
			require.Equal(t, []string{"kafka", "log"}, order)
		})
	}
}
func TestKafkaDrainWaitsForRun(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	s := startKafka(fakeKafka{
		run: func(ctx context.Context, _ kafka.Handler) error { close(started); <-release; return ctx.Err() },
		stop: func(ctx context.Context) error {
			_, ok := ctx.Deadline()
			if !ok {
				return errors.New("missing deadline")
			}
			close(release)
			return nil
		},
	}, nil, time.Second)
	<-started
	require.NoError(t, s.Shutdown(context.Background()))
	select {
	case <-s.finished:
	default:
		t.Fatal("Run must finish before logs close")
	}
}
func TestKafkaDrainTimeout(t *testing.T) {
	release := make(chan struct{})
	s := startKafka(fakeKafka{
		run:  func(context.Context, kafka.Handler) error { <-release; return nil },
		stop: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	}, nil, 10*time.Millisecond)
	require.ErrorIs(t, s.Shutdown(context.Background()), context.DeadlineExceeded)
	close(release)
	<-s.finished
}
func TestKafkaConstructAndClose(t *testing.T) {
	o := kafka.DefaultOptions()
	o.Brokers = []string{"127.0.0.1:1"}
	o.Topics = []string{"example"}
	o.GroupID = "test"
	o.StartOffset = "earliest"
	s := MustKafkaWorker(global.KafkaConfig{Enabled: true, Options: o, Drain: time.Second}, func(context.Context, kafka.Message) error { return nil }, closedRuntime(t))
	require.NotNil(t, global.KafkaCli)
	require.NoError(t, s.Shutdown(context.Background()))
	global.KafkaCli = nil
}
