package bootstrap

import (
	"context"
	"net"
	"serv_tpl/internal/telemetry"
	"time"
)

// startMetrics 同步绑定独立监控端口；错误交给观测装配降级处理。
func startMetrics(r *telemetry.Runtime) (*HTTPService, error) {
	if !r.Options.Metrics.Enable {
		return nil, nil
	}
	// DNS 解析也在期限内，错误的监控主机名不能无限延迟业务启动。
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return listenMetrics(ctx, r, (&net.ListenConfig{}).Listen)
}

func listenMetrics(ctx context.Context, r *telemetry.Runtime, listen func(context.Context, string, string) (net.Listener, error)) (*HTTPService, error) {
	ln, err := listen(ctx, "tcp", r.Options.Metrics.Address)
	if err != nil {
		return nil, err
	}
	return serveHTTP(r.Handler(), httpOptions{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, DrainTimeout: r.Options.Metrics.ShutdownTimeout}, ln), nil
}
