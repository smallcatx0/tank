package bootstrap

import (
	"context"
	"errors"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"serv_tpl/pkg/logger"
	"sync/atomic"
	"syscall"
	"time"
)

// Cleanup 描述一个按资源逆序执行的关闭动作。
type Cleanup struct {
	Name string
	Stop func(context.Context) error
}

// Runtime 持有进程生命周期；Add 仅在启动线程调用。
type Runtime struct {
	ctx      context.Context
	cancel   context.CancelFunc
	cleanups []Cleanup
	ready    atomic.Bool
	failed   atomic.Bool
	timeout  atomic.Int64
}

// Add 登记资源；关闭时按登记逆序执行，日志应最先登记。
func (r *Runtime) Add(name string, stop func(context.Context) error) {
	r.cleanups = append(r.cleanups, Cleanup{name, stop})
}

// Ready 返回生命周期就绪状态，不代替下游健康探测。
func (r *Runtime) Ready() bool { return r.ready.Load() && r.ctx.Err() == nil }

// SetShutdownTimeout 设置配置校验后的总退出期限。
func (r *Runtime) SetShutdownTimeout(d time.Duration) { r.timeout.Store(int64(d)) }

// Fail 将后台致命故障传给运行编排；不在故障线程阻塞写日志。
func (r *Runtime) Fail() { r.failed.Store(true); r.ready.Store(false); r.cancel() }

// Run 在初始化前监听信号，统一处理失败、排空与强制退出。
func Run(setup func(context.Context, *Runtime) error) int {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(signals)
	return run(signals, os.Exit, setup)
}
func run(signals <-chan os.Signal, forceExit func(int), setup func(context.Context, *Runtime) error) (code int) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{ctx: ctx, cancel: cancel}
	r.SetShutdownTimeout(30 * time.Second)
	finished := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		defer func() {
			if recover() != nil {
				forceExit(1)
			}
		}()
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		case <-finished:
			return
		}
		r.ready.Store(false)
		timer := time.NewTimer(time.Duration(r.timeout.Load()))
		defer timer.Stop()
		// 步骤1：强退路径不调用日志 Sync，避免日志阻塞看门狗。
		select {
		case <-signals:
			forceExit(1)
		case <-timer.C:
			forceExit(1)
		case <-finished:
		}
	}()
	defer func() {
		if recover() != nil {
			r.Fail()
			code = 1
			logPanic("startup")
		}
		r.ready.Store(false)
		cancel()
		cleanupCtx, stop := context.WithTimeout(context.Background(), time.Duration(r.timeout.Load()))
		defer stop()
		for i := len(r.cleanups) - 1; i >= 0; i-- {
			if err := runCleanup(r.cleanups[i], cleanupCtx); err != nil {
				code = 1
			}
		}
		if r.failed.Load() || cleanupCtx.Err() != nil {
			code = 1
		}
		close(finished)
		<-watcherDone
	}()
	if err := setup(ctx, r); err != nil {
		r.Fail()
		logger.L().Error("服务初始化失败", zap.String("component", "startup"), zap.Error(err))
		return 1
	}
	if ctx.Err() != nil {
		return 1
	}
	r.ready.Store(true)
	<-ctx.Done()
	return 0
}
func runCleanup(c Cleanup, ctx context.Context) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("清理 panic")
			if c.Name != "log" {
				logPanic(c.Name)
			}
		}
	}()
	if c.Stop == nil {
		return nil
	}
	err = c.Stop(ctx)
	if err != nil && c.Name != "log" {
		logger.L().Error("清理失败", zap.String("component", c.Name))
	}
	return err
}
