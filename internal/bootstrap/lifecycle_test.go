package bootstrap

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestWaitingExitOrder(t *testing.T) {
	for _, mode := range []string{"http", "closed-http", "cleanup-error", "cleanup-panic", "log-panic"} {
		t.Run(mode, func(t *testing.T) {
			httpDone := make(chan error, 1)
			switch mode {
			case "http":
				httpDone <- errors.New("serve failed")
			case "closed-http":
				close(httpDone)
			default:
				httpDone <- errors.New("serve failed")
			}
			var order []string
			code := WaitingExit(5*time.Second, httpDone,
				Cleanup{"http", func(ctx context.Context) error {
					require.NoError(t, ctx.Err())
					order = append(order, "http")
					if mode == "cleanup-error" {
						return errors.New("failure")
					}
					if mode == "cleanup-panic" {
						panic("secret")
					}
					return nil
				}},
				Cleanup{"storage", func(context.Context) error { order = append(order, "storage"); return nil }},
				Cleanup{"log", func(context.Context) error {
					order = append(order, "log")
					if mode == "log-panic" {
						panic("log")
					}
					return nil
				}},
			)
			require.Equal(t, 1, code)
			require.Equal(t, []string{"http", "storage", "log"}, order)
		})
	}
}
func TestMustStartupFailures(t *testing.T) {
	require.Panics(t, func() { MustInitDBs(0) })
}

func TestCloseResourceFailure(t *testing.T) {
	for _, name := range []string{"http", "log"} {
		t.Run(name, func(t *testing.T) {
			failure := errors.New("close failure")
			require.ErrorIs(t, closeResource(context.Background(), name, func(context.Context) error { return failure }), failure)
			require.Error(t, closeResource(context.Background(), name, func(context.Context) error { panic("close panic") }))
			require.NoError(t, closeResource(context.Background(), name, nil))
		})
	}
}
