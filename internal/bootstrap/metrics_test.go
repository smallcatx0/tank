package bootstrap

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"serv_tpl/internal/telemetry"
	"testing"
	"time"
)

func TestMetricsLifecycle(t *testing.T) {
	off, err := telemetry.New(context.Background(), telemetry.Options{})
	require.NoError(t, err)
	s, err := startMetrics(off)
	require.NoError(t, err)
	require.Nil(t, s)
	require.Nil(t, s.Done())
	require.NoError(t, s.Shutdown(context.Background()))
	r, err := telemetry.New(context.Background(), telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: true, Address: "127.0.0.1:0", ShutdownTimeout: time.Second}})
	require.NoError(t, err)
	s, err = startMetrics(r)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + s.listener.Addr().String() + "/metrics")
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	r.Options.Metrics.Address = s.listener.Addr().String()
	_, err = startMetrics(r)
	require.Error(t, err)
	require.NoError(t, s.listener.Close())
	require.Equal(t, 1, WaitingExit(3*time.Second, nil, nil, s.Done(), Cleanup{Name: "metrics", Stop: s.Shutdown}))
	closed := make(chan error)
	close(closed)
	require.Equal(t, 1, WaitingWorkerExit(time.Second, nil, closed))
}
