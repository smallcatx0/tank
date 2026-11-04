package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"serv_tpl/internal/global"
	"serv_tpl/internal/httpmd"
	"serv_tpl/routes"

	"github.com/gin-gonic/gin"

	"net"
	"net/http"

	"sync"
	"sync/atomic"
	"time"
)

// httpOptions 定义包内 HTTP 运行与排空的各阶段期限。
type httpOptions struct {
	ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout, DrainTimeout time.Duration
}

// MustStartHTTP 装配本服务的中间件和路由，同步监听后发布就绪，失败直接 panic。
func MustStartHTTP(cfg global.LifecycleConfig) *HTTPService {
	// 步骤1：设置模式并在监听前完成全部路由装配。
	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	ready := new(atomic.Bool)
	engine := gin.New()
	engine.Use(httpmd.Trace, httpmd.Recovery)

	routes.RegisterHTTP(engine, ready.Load)

	// 步骤2：监听成功后发布就绪，关闭时由 HTTPService 撤销。
	opts := httpOptions{
		ReadHeaderTimeout: cfg.ReadHeader, ReadTimeout: cfg.Read,
		WriteTimeout: cfg.Write, IdleTimeout: cfg.Idle, DrainTimeout: cfg.Drain,
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		panic("HTTP 监听失败")
	}
	s := serveHTTP(engine, opts, ln)
	s.ready = ready
	ready.Store(true)
	return s
}

// HTTPService 持有监听器和请求生命周期，不绑定退出信号直接取消请求。
type HTTPService struct {
	server   *http.Server
	listener net.Listener
	cancel   context.CancelFunc
	done     chan error
	finished chan struct{}
	stopping atomic.Bool
	ready    *atomic.Bool // 由 MustStartHTTP 创建，与健康路由共享。
	once     sync.Once
	err      error
	serveErr error // finished 关闭后可读，避免信号与 Serve 故障竞态丢失错误。
	drain    time.Duration
}

// serveHTTP 在已创建的监听器上运行服务，管理请求生命周期与 Serve 故障通知。
func serveHTTP(handler http.Handler, o httpOptions, ln net.Listener) *HTTPService {
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
			s.serveErr = err
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

// Shutdown 先撤销就绪再幂等排空；超时取消请求并强制关闭连接。
func (s *HTTPService) Shutdown(ctx context.Context) error {
	s.once.Do(func() {
		if s.ready != nil {
			s.ready.Store(false)
		}
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
			s.err = errors.Join(s.err, s.serveErr)
		case <-ctx.Done():
			s.err = errors.Join(s.err, ctx.Err())
		}
	})
	return s.err
}
