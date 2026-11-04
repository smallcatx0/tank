package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"serv_tpl/pkg/logger"
)

// Cleanup 是运行结束后的一个关闭步骤；调用方按执行顺序传入，日志必须最后。
type Cleanup struct {
	Name string
	Stop func(context.Context) error
}

// WaitingExit 仅在启动成功后调用，等待信号或 HTTP 停止并按顺序关闭资源。
// 无 HTTP 服务时 httpDone 传 nil；不接收启动回调、不恢复启动 panic，也不回滚启动资源。
func WaitingExit(timeout time.Duration, httpDone <-chan error, steps ...Cleanup) int {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(signals)
	var failure error
	select {
	case <-signals:
	case failure = <-httpDone:
		if failure == nil {
			failure = errors.New("HTTP 意外停止")
		}
	}
	// 步骤1：在记录故障和关闭资源前启动独立看门狗，日志阻塞也能强退。
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	finished, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recover() != nil {
				os.Exit(1)
			}
		}()
		select {
		case <-signals:
			os.Exit(1)
		case <-ctx.Done():
			os.Exit(1)
		case <-finished:
		}
	}()
	defer func() { close(finished); <-done }()

	code := 0
	if failure != nil {
		code = 1
	}
	// 步骤2：逐项隔离关闭异常并继续后续步骤，运行故障在日志关闭前记录。
	for _, step := range steps {
		if step.Name == "log" && failure != nil {
			logger.L().Error("服务运行失败", zap.Error(failure))
		}
		if err := closeResource(ctx, step.Name, step.Stop); err != nil {
			code = 1
		}
	}
	if ctx.Err() != nil {
		code = 1
	}
	return code
}

func closeResource(ctx context.Context, name string, stop func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("清理 panic")
			if name != "log" {
				logPanic(name)
			}
		}
	}()
	if stop == nil {
		return nil
	}
	err = stop(ctx)
	if err != nil && name != "log" {
		logger.L().Error("清理失败", zap.String("component", name))
	}
	return err
}
