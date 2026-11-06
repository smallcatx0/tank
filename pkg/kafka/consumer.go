package kafka

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"serv_tpl/pkg/logger"
)

type state uint8

const (
	created state = iota
	running
	draining
	stopping
	closed
)

type partitionKey struct {
	topic     string
	partition int32
}

// Consumer 持有单次运行的消费者及其关闭状态；使用 NewConsumer 构造。
type Consumer struct {
	options    Options
	client     client
	mu         sync.Mutex
	state      state
	err        error
	owned      map[partitionKey]bool
	ctx        context.Context
	runValues  context.Context
	cancel     context.CancelCauseFunc
	pollCtx    context.Context
	pollCancel context.CancelFunc
	done       chan struct{}
	commitGate chan struct{}
	metrics    metrics
}

func newConsumer(o Options) *Consumer {
	ctx, cancel := context.WithCancelCause(context.Background())
	pollCtx, pollCancel := context.WithCancel(ctx)
	return &Consumer{options: o, ctx: ctx, cancel: cancel, pollCtx: pollCtx, pollCancel: pollCancel, done: make(chan struct{}), commitGate: make(chan struct{}, 1), owned: make(map[partitionKey]bool), metrics: newMetrics(o)}
}

// Run 阻塞消费，返回前等待全部处理与客户端关闭；不允许再次启动。
func (c *Consumer) Run(ctx context.Context, handler Handler) (result error) {
	if ctx == nil || handler == nil {
		return fmt.Errorf("kafka: Run 需要非 nil context 和 Handler")
	}
	c.mu.Lock()
	if c.state == closed {
		c.mu.Unlock()
		return ErrClosed
	}
	if c.state != created {
		c.mu.Unlock()
		return ErrAlreadyRun
	}
	c.state = running
	c.runValues = ctx
	c.mu.Unlock()
	// 步骤1：取消监听只更新状态；返回前等待回调结束，避免退出后仍有内部任务。
	watched := make(chan struct{})
	unwatch := context.AfterFunc(ctx, func() {
		defer close(watched)
		defer func() {
			if p := recover(); p != nil {
				c.stop(c.panicError("group", p))
			}
		}()
		c.stop(ctx.Err())
	})
	defer func() {
		if p := recover(); p != nil {
			c.stop(c.panicError("fetch", p))
		}
		if ctx.Err() != nil {
			c.stop(ctx.Err())
		}
		c.finish()
		if !unwatch() {
			<-watched
		}
		result = c.result()
	}()
	for {
		c.mu.Lock()
		active := c.state == running
		c.mu.Unlock()
		if !active || ctx.Err() != nil {
			return nil
		}
		poll, cancel := context.WithTimeout(c.pollCtx, c.options.Advanced.PollTimeout)
		fetches := c.client.PollRecords(poll, c.options.Advanced.MaxPollRecords)
		pollErr := poll.Err()
		cancel()
		for _, e := range fetches.Errors() {
			if (errors.Is(e.Err, context.DeadlineExceeded) && errors.Is(pollErr, context.DeadlineExceeded) && fetches.NumRecords() == 0) || (errors.Is(e.Err, context.Canceled) && c.pollCtx.Err() != nil) {
				continue
			}
			c.stop(&ConsumeError{Stage: "fetch", Topic: e.Topic, Partition: e.Partition, Offset: -1, Err: e.Err})
			return nil
		}
		if c.ctx.Err() != nil {
			return nil
		}
		if fetches.NumRecords() > 0 {
			if err := c.round(fetches, handler); err != nil {
				c.stop(err)
				return nil
			}
		}
		c.client.AllowRebalance()
	}
}

func (c *Consumer) round(fetches kgo.Fetches, handler Handler) error {
	budget := timeDurationBudget(c.options)
	ctx, cancel := context.WithTimeout(valueContext{c.ctx, c.runValues}, budget)
	defer cancel()
	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(c.options.MaxConcurrency)
	c.metrics.pending.Set(float64(fetches.NumRecords()))
	defer c.metrics.pending.Set(0)
	// 步骤2：每个分区一个有序任务，跨分区共享有界工作槽。
	fetches.EachPartition(func(p kgo.FetchTopicPartition) {
		if len(p.Records) == 0 {
			return
		}
		group.Go(func() (err error) {
			defer func() {
				if v := recover(); v != nil {
					err = c.panicError("handle", v)
					c.stop(err)
				}
			}()
			for _, r := range p.Records {
				c.mu.Lock()
				if c.state != running || gctx.Err() != nil || (c.runValues != nil && c.runValues.Err() != nil) {
					c.mu.Unlock()
					return nil
				}
				c.metrics.inflight.Inc()
				c.metrics.pending.Dec()
				c.mu.Unlock()
				err = c.process(gctx, r, handler)
				c.metrics.inflight.Dec()
				if err != nil {
					c.stop(err)
					return err
				}
			}
			return nil
		})
	})
	err := group.Wait()
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return nil
}

func (c *Consumer) stop(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == closed {
		return
	}
	if c.err == nil {
		c.err = err
	}
	// 构造后、Run 前也可能收到连接初始化错误；保留 created 的清理所有权，
	// 由随后第一次 Run 或 Shutdown 真正关闭客户端。
	if c.state != created {
		c.state = stopping
	}
	c.cancel(c.err)
	c.pollCancel()
}

// Shutdown 停止新消息并排空在途消息；要求独立且带 deadline 的 context。
func (c *Consumer) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("kafka: Shutdown context 不能为 nil")
	}
	if _, ok := ctx.Deadline(); !ok {
		return fmt.Errorf("kafka: Shutdown 必须携带 deadline")
	}
	c.mu.Lock()
	if c.state == closed {
		err := c.err
		c.mu.Unlock()
		return err
	}
	wasCreated := c.state == created
	if c.state == created || c.state == running {
		c.state = draining
	}
	c.pollCancel()
	c.mu.Unlock()
	if ctx.Err() != nil {
		c.stop(ctx.Err())
	}
	if wasCreated {
		// 步骤1：尚未运行时由关闭任务持有资源，done 只在真实清理完成后关闭。
		go c.finish()
	}
	select {
	case <-c.done:
		return c.result()
	case <-ctx.Done():
		c.stop(ctx.Err())
		c.metrics.events.WithLabelValues("shutdown_failure").Inc()
		return errors.Join(ctx.Err(), c.result())
	}
}

func (c *Consumer) finish() {
	// 每个拥有资源的执行路径只调用一次；即使关闭 panic 也发布终态。
	defer func() {
		if p := recover(); p != nil {
			err := c.panicError("close", p)
			c.mu.Lock()
			c.err = errors.Join(c.err, err)
			c.mu.Unlock()
		}
		c.mu.Lock()
		c.state = closed
		c.cancel(c.err)
		c.pollCancel()
		close(c.done)
		c.mu.Unlock()
	}()
	c.client.CloseAllowingRebalance()
}

// valueContext 沿用 Run 的业务上下文值，由消费者生命周期控制取消。
type valueContext struct {
	context.Context
	values context.Context
}

func (v valueContext) Value(key any) any {
	if value := v.Context.Value(key); value != nil {
		return value
	}
	if v.values != nil {
		return v.values.Value(key)
	}
	return v.Context.Value(key)
}

func (c *Consumer) result() error { c.mu.Lock(); defer c.mu.Unlock(); return c.err }
func (c *Consumer) panicError(stage string, p any) error {
	err := &ConsumeError{Stage: stage, Partition: -1, Offset: -1, Err: fmt.Errorf("panic (%T)", p)}
	logger.L().Error("Kafka 执行 panic", zap.String("consumer", c.options.ClientID), zap.String("group", c.options.GroupID), zap.Error(err), zap.ByteString("stack", debug.Stack()))
	return err
}
