package bootstrap

import (
	"context"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"net"
	"serv_tpl/internal/global"
	"serv_tpl/internal/telemetry"
	"serv_tpl/pkg/logger"
	"strconv"
	"sync"
	"time"
)

// Observability 持有可选观测资源；Runtime 总是可用，包括初始化失败后的关闭态。
type Observability struct {
	Runtime *telemetry.Runtime
	monitor *HTTPService
	watched chan struct{}
	once    sync.Once
}

// StartObservability 按配置启动心跳、指标和追踪，错误仅警告，不阻止业务启动。
// cfg/drains 来自已校验的业务配置，用于保留业务端口并校验退出预算。
func StartObservability(c *viper.Viper, cfg global.LifecycleConfig, drains ...time.Duration) *Observability {
	startHeartbeat(c)
	options, err := global.LoadObservabilityConfig(c)
	if err != nil {
		warnObservation("config")
		options = telemetry.Options{}
	}
	if options.Metrics.Enable && conflictsBusinessPort(options.Metrics.Address, cfg) {
		warnObservation("metrics_business_port")
		options.Metrics.Enable = false
	}
	if err = global.ValidateObservabilityBudget(options, cfg.Shutdown, drains...); err != nil {
		warnObservation("shutdown_budget")
		options = telemetry.Options{}
	}
	return startObservation(options, telemetry.New)
}

// 同端口保守让位于业务，即便不同绑定地址可能共存，也不让可选监听抢占业务端口。
func conflictsBusinessPort(address string, cfg global.LifecycleConfig) bool {
	_, port, _ := net.SplitHostPort(address)
	n, _ := strconv.Atoi(port)
	if cfg.Port > 0 && n == cfg.Port {
		return true
	}
	if cfg.GRPC.Enabled {
		_, rpcPort, _ := net.SplitHostPort(cfg.GRPC.Address)
		rpc, _ := strconv.Atoi(rpcPort)
		return rpc > 0 && n == rpc
	}
	return false
}

func startObservation(options telemetry.Options, newRuntime func(context.Context, telemetry.Options) (*telemetry.Runtime, error)) *Observability {
	r, err := newRuntime(context.Background(), options)
	if err != nil {
		warnObservation("initialize")
		// 全关闭配置只创建无后台任务的 Provider/Propagator，不存在构造失败分支。
		r, _ = telemetry.New(context.Background(), telemetry.Options{})
	}
	o := &Observability{Runtime: r, watched: make(chan struct{})}
	o.monitor, err = startMetrics(r)
	if err != nil {
		warnObservation("metrics_listen")
		// 仅在发布给业务之前调整配置，保留可用的追踪能力。
		r.Options.Metrics.Enable = false
		r.Registry = nil
		r.Metrics = nil
	}
	if o.monitor == nil {
		close(o.watched)
		return o
	}
	go func() {
		defer close(o.watched)
		defer func() {
			if recover() != nil {
				logPanic("metrics_observer")
			}
		}()
		if err := <-o.monitor.Done(); err != nil {
			warnObservation("metrics_serve")
		}
	}()
	return o
}

// Shutdown 幂等释放观测资源；阶段错误只警告，业务退出码不受观测失败影响。
func (o *Observability) Shutdown(ctx context.Context) error {
	o.once.Do(func() {
		// 给后续日志关闭保留余量；不能为可选 flush 耗尽进程总退出期限。
		stage := ctx
		if deadline, ok := ctx.Deadline(); ok {
			var cancel context.CancelFunc
			stage, cancel = context.WithDeadline(ctx, deadline.Add(-time.Second))
			defer cancel()
		}
		if err := o.monitor.Shutdown(stage); err != nil {
			warnObservation("metrics_shutdown")
		}
		select {
		case <-o.watched:
		case <-stage.Done():
			warnObservation("metrics_wait")
		}
		if err := o.Runtime.Shutdown(stage); err != nil {
			warnObservation("tracing_shutdown")
		}
	})
	return nil
}

// 观测错误可能携带导出地址或凭据，只记录固定失败阶段。
func warnObservation(stage string) {
	logger.L().Warn("可选观测不可用，业务继续运行", zap.String("component", "observability"), zap.String("stage", stage))
}
