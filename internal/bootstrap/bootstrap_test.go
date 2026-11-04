package bootstrap

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"serv_tpl/internal/global"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// async 让测试 goroutine 的意外 panic 可观察。
func async(t *testing.T, fn func()) {
	t.Helper()
	go func() {
		defer func() {
			if v := recover(); v != nil {
				t.Errorf("测试 goroutine panic: %v", v)
			}
		}()
		fn()
	}()
}
func TestCleanup(t *testing.T) {
	require.NoError(t, runCleanup(Cleanup{}, context.Background()))
	require.Error(t, runCleanup(Cleanup{"bad", func(context.Context) error { panic("private") }}, context.Background()))
	require.Error(t, runCleanup(Cleanup{"log", func(context.Context) error { panic("private") }}, context.Background()))
	require.Error(t, runCleanup(Cleanup{"error", func(context.Context) error { return errors.New("fail") }}, context.Background()))
}
func TestRuntimeOrder(t *testing.T) {
	signals := make(chan os.Signal, 2)
	entered := make(chan *Runtime, 1)
	result := make(chan int, 1)
	var order []string
	async(t, func() {
		result <- run(signals, func(int) { t.Error("unexpected forced exit") }, func(ctx context.Context, r *Runtime) error {
			r.Add("log", func(context.Context) error { order = append(order, "log"); return nil })
			r.Add("db", func(context.Context) error { order = append(order, "db"); return nil })
			r.Add("http", func(context.Context) error { require.False(t, r.Ready()); order = append(order, "http"); return nil })
			entered <- r
			return nil
		})
	})
	r := <-entered
	require.Eventually(t, r.Ready, time.Second, time.Millisecond)
	signals <- syscall.SIGTERM
	require.Equal(t, 0, <-result)
	require.Equal(t, []string{"http", "db", "log"}, order)
}
func TestRuntimeFailures(t *testing.T) {
	for _, mode := range []string{"setup-error", "setup-panic", "background", "cleanup-error", "cleanup-panic"} {
		t.Run(mode, func(t *testing.T) {
			var cleaned bool
			code := run(make(chan os.Signal), func(int) { t.Error("forced") }, func(ctx context.Context, r *Runtime) error {
				r.Add("log", func(context.Context) error { cleaned = true; return nil })
				switch mode {
				case "setup-error":
					return errors.New("fail")
				case "setup-panic":
					panic("secret")
				case "cleanup-error":
					r.Add("bad", func(context.Context) error { return errors.New("fail") })
				case "cleanup-panic":
					r.Add("bad", func(context.Context) error { panic("fail") })
				}
				r.Fail()
				return nil
			})
			require.Equal(t, 1, code)
			require.True(t, cleaned)
		})
	}
}
func TestRuntimeWatchdog(t *testing.T) {
	for _, second := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "second-signal"}[second], func(t *testing.T) {
			signals := make(chan os.Signal, 2)
			blocked := make(chan struct{})
			entered := make(chan struct{})
			forced := make(chan int, 1)
			result := make(chan int, 1)
			async(t, func() {
				result <- run(signals, func(n int) { forced <- n }, func(ctx context.Context, r *Runtime) error {
					r.SetShutdownTimeout(40 * time.Millisecond)
					r.Add("log", func(context.Context) error { close(entered); <-blocked; return nil })
					return errors.New("trigger")
				})
			})
			<-entered
			if second {
				signals <- syscall.SIGTERM
			}
			select {
			case n := <-forced:
				require.Equal(t, 1, n)
			case <-time.After(time.Second):
				t.Fatal("watchdog blocked")
			}
			close(blocked)
			require.Equal(t, 1, <-result)
		})
	}
}
func TestRuntimeStartupSignal(t *testing.T) {
	sig := make(chan os.Signal, 2)
	require.Equal(t, 1, run(sig, func(int) { t.Error("forced") }, func(ctx context.Context, r *Runtime) error { sig <- os.Interrupt; <-ctx.Done(); return ctx.Err() }))
}
func TestInitDBRollback(t *testing.T) {
	oldDB, oldRedis := global.MysqlCli, global.RedisCli
	defer func() { global.MysqlCli, global.RedisCli = oldDB, oldRedis }()
	for _, mode := range []string{"db-fail", "redis-fail", "canceled", "success"} {
		t.Run(mode, func(t *testing.T) {
			var order []string
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closeFn, err := initDBs(ctx, func(context.Context) (*gorm.DB, error) {
				if mode == "db-fail" {
					return nil, errors.New("db")
				}
				return &gorm.DB{}, nil
			},
				func(context.Context) (*redis.Client, error) {
					if mode == "redis-fail" {
						return nil, errors.New("redis")
					}
					if mode == "canceled" {
						cancel()
					}
					return &redis.Client{}, nil
				},
				func(*gorm.DB) error { order = append(order, "mysql"); return nil }, func(*redis.Client) error { order = append(order, "redis"); return nil })
			if mode == "success" {
				require.NoError(t, err)
				require.NoError(t, closeFn(ctx))
				require.NoError(t, closeFn(ctx))
			} else {
				require.Error(t, err)
			}
			switch mode {
			case "db-fail":
				require.Empty(t, order)
			case "redis-fail":
				require.Equal(t, []string{"mysql"}, order)
			default:
				require.Equal(t, []string{"redis", "mysql"}, order)
			}
		})
	}
}
func TestLogInit(t *testing.T) {
	old := global.AppConf
	defer func() { global.AppConf = old }()
	for _, medium := range []string{"stdout", "file"} {
		global.AppConf = viper.New()
		global.AppConf.Set("log.type", medium)
		global.AppConf.Set("log.path", t.TempDir()+"/nested/app.log")
		stop := MustInitLog()
		require.NoError(t, stop(context.Background()))
		require.NoError(t, stop(context.Background()))
	}
	global.AppConf = viper.New()
	global.AppConf.Set("log.type", "file")
	require.Panics(t, func() { MustInitLog() })
	global.AppConf = viper.New()
	stop := MustInitLog()
	require.NoError(t, stop(context.Background()))
}
func TestHeartbeat(t *testing.T) {
	stop, err := StartHeartbeat(HeartbeatOptions{})
	require.NoError(t, err)
	require.NoError(t, stop(context.Background()))
	_, err = StartHeartbeat(HeartbeatOptions{Enabled: true})
	require.Error(t, err)
	var count atomic.Int32
	stop, err = startHeartbeat(HeartbeatOptions{Enabled: true, Interval: time.Millisecond}, func(context.Context) { count.Add(1) })
	require.NoError(t, err)
	require.Eventually(t, func() bool { return count.Load() > 0 }, time.Second, time.Millisecond)
	require.NoError(t, stop(context.Background()))
	n := count.Load()
	require.NoError(t, stop(context.Background()))
	require.Equal(t, n, count.Load())
	fail := make(chan struct{})
	var once sync.Once
	stop, err = startHeartbeat(HeartbeatOptions{Enabled: true, Interval: time.Millisecond, OnFailure: func() { once.Do(func() { close(fail) }) }}, func(context.Context) { panic("secret") })
	require.NoError(t, err)
	<-fail
	require.NoError(t, stop(context.Background()))
}
func TestHeartbeatCancelSampling(t *testing.T) {
	entered := make(chan struct{})
	stop, err := startHeartbeat(HeartbeatOptions{Enabled: true, Interval: time.Millisecond}, func(ctx context.Context) { close(entered); <-ctx.Done() })
	require.NoError(t, err)
	<-entered
	require.NoError(t, stop(context.Background()))
}

func TestInitDBPanicRollback(t *testing.T) {
	closed := false
	require.Panics(t, func() {
		_, _ = initDBs(context.Background(),
			func(context.Context) (*gorm.DB, error) { return &gorm.DB{}, nil },
			func(context.Context) (*redis.Client, error) { panic("constructor") },
			func(*gorm.DB) error { closed = true; return nil }, func(*redis.Client) error { return nil })
	})
	require.True(t, closed)
}
