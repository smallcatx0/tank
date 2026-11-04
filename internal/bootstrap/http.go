package bootstrap

import (
	"context"
	"errors"
	"go.uber.org/zap"
	"net"
	"net/http"
	"serv_tpl/pkg/logger"
	"sync"
	"sync/atomic"
	"time"
)

// HTTPOptions 定义监听地址与各阶段期限。
type HTTPOptions struct {
	Address                                                                 string
	ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout, DrainTimeout time.Duration
}

// HTTPService 持有监听器和请求生命周期，不绑定退出信号直接取消请求。
type HTTPService struct {
	server   *http.Server
	listener net.Listener
	cancel   context.CancelFunc
	done     chan error
	finished chan struct{}
	stopping atomic.Bool
	once     sync.Once
	err      error
	drain    time.Duration
}

// StartHTTP 同步监听，所有路由必须已装配；失败不遗留后台任务。
func StartHTTP(handler http.Handler, o HTTPOptions) (*HTTPService, error) {
	ln, err := net.Listen("tcp", o.Address)
	if err != nil {
		return nil, err
	}
	return serveHTTP(handler, o, ln), nil
}
func serveHTTP(handler http.Handler, o HTTPOptions, ln net.Listener) *HTTPService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &HTTPService{listener: ln, cancel: cancel, done: make(chan error, 1), finished: make(chan struct{}), drain: o.DrainTimeout}
	s.server = &http.Server{Handler: handler, ReadHeaderTimeout: o.ReadHeaderTimeout, ReadTimeout: o.ReadTimeout, WriteTimeout: o.WriteTimeout, IdleTimeout: o.IdleTimeout, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		var err error
		defer close(s.finished)
		defer func() {
			panicked := recover() != nil
			if panicked {
				err = errors.New("HTTP Serve panic")
			}
			s.done <- err
			if panicked {
				logPanic("http")
			}
			close(s.done)
		}()
		err = s.server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) && s.stopping.Load() {
			err = nil
		}
	}()
	return s
}

// Done 返回 Serve 的唯一结果，供运行编排监测。
func (s *HTTPService) Done() <-chan error { return s.done }

// Shutdown 幂等排空；超时取消请求并强制关闭连接。
func (s *HTTPService) Shutdown(ctx context.Context) error {
	s.once.Do(func() {
		s.stopping.Store(true)
		drain, cancel := context.WithTimeout(ctx, s.drain)
		defer cancel()
		s.err = s.server.Shutdown(drain)
		if s.err != nil {
			s.cancel()
			s.err = errors.Join(s.err, s.server.Close())
		}
		s.cancel()
		select {
		case <-s.finished:
		case <-ctx.Done():
			s.err = errors.Join(s.err, ctx.Err())
		}
	})
	return s.err
}

// WatchHTTP 监测 Serve 结果；异常退出触发进程失败。
func (r *Runtime) WatchHTTP(s *HTTPService) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recover() != nil {
				r.Fail()
				logPanic("http-watch")
			}
		}()
		if err := <-s.Done(); err != nil {
			r.Fail()
			logger.L().Error("HTTP 服务异常退出", zap.String("component", "http"), zap.Error(err))
		}
	}()
	// 步骤1：HTTP 清理同时等待监测协程结束，再允许关闭日志。
	r.Add("http", func(ctx context.Context) error {
		err := s.Shutdown(ctx)
		select {
		case <-done:
			return err
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
	})
}
