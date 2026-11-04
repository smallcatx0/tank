package bootstrap

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestWaitingExitOrder(t *testing.T) {
	for _, mode := range []string{"http", "grpc", "closed-http", "closed-grpc", "nil-http-result", "nil-grpc-result", "nil-http-channel", "cleanup-error", "cleanup-panic", "log-panic"} {
		t.Run(mode, func(t *testing.T) {
			httpDone := make(chan error, 1)
			switch mode {
			case "http":
				httpDone <- errors.New("serve failed")
			case "closed-http", "closed-grpc":
				close(httpDone)
			case "nil-http-result", "nil-grpc-result":
				httpDone <- nil
			default:
				httpDone <- errors.New("serve failed")
			}
			var order []string
			var grpcDone <-chan error
			var httpResult <-chan error = httpDone
			if mode == "grpc" || mode == "closed-grpc" || mode == "nil-grpc-result" {
				grpcDone, httpResult = httpDone, make(chan error)
			}
			if mode == "nil-http-channel" {
				grpcDone, httpResult = httpDone, nil
			}
			code := WaitingExit(5*time.Second, httpResult, grpcDone,
				Cleanup{Name: "http", Stop: func(ctx context.Context) error {
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
				Cleanup{Name: "grpc"},
				Cleanup{Name: "storage", Stop: func(context.Context) error { order = append(order, "storage"); return nil }},
				Cleanup{Name: "log", Stop: func(context.Context) error {
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
