package bootstrap

import (
	"context"
	"errors"
	"serv_tpl/pkg/logger"
	"sync"
	"time"
)

// HeartbeatOptions 定义是否采样、间隔及致命异常通知。
type HeartbeatOptions struct {
	Enabled   bool
	Interval  time.Duration
	OnFailure func()
}

// StartHeartbeat 启动可停止、可等待的系统采样，不把采样放在请求路径。
func StartHeartbeat(o HeartbeatOptions) (func(context.Context) error, error) {
	return startHeartbeat(o, func(ctx context.Context) { logger.SysStatInfoContext(ctx, logger.L()) })
}
func startHeartbeat(o HeartbeatOptions, sample func(context.Context)) (func(context.Context) error, error) {
	if !o.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	if o.Interval <= 0 {
		return nil, errors.New("心跳间隔必须为正")
	}
	ctx, cancel := context.WithCancel(context.Background())
	stop := ctx.Done()
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		defer func() {
			if recover() != nil {
				if o.OnFailure != nil {
					o.OnFailure()
				}
				logPanic("heartbeat")
			}
		}()
		ticker := time.NewTicker(o.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				select {
				case <-stop:
					return
				default:
					sample(ctx)
				}
			}
		}
	}()
	return func(ctx context.Context) error {
		once.Do(cancel)
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, nil
}
