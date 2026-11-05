package kafka

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"go.uber.org/zap"
	"serv_tpl/pkg/logger"
)

func timeDurationBudget(o Options) time.Duration {
	return time.Duration(o.Advanced.MaxPollRecords) * (o.MessageTimeout + o.CommitTimeout)
}

func backoff(initial, maximum time.Duration, n int) time.Duration {
	base := initial
	for i := 1; i < n && base < maximum; i++ {
		if base > maximum/2 {
			base = maximum
		} else {
			base *= 2
		}
	}
	if base > maximum {
		base = maximum
	}
	// 除法先行，避免计算百分比时溢出；区间封顶到配置上限。
	span := base / 5
	return base - span + time.Duration(rand.Int64N(int64(span)+1))
}

func message(r *kgo.Record) Message {
	var headers []Header
	if r.Headers != nil {
		headers = make([]Header, len(r.Headers))
		for i, h := range r.Headers {
			headers[i] = Header{h.Key, h.Value}
		}
	}
	return Message{Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Key: r.Key, Value: r.Value, Headers: headers, Timestamp: r.Timestamp}
}

func (c *Consumer) process(ctx context.Context, r *kgo.Record, handler Handler) error {
	started := time.Now()
	msgCtx, cancel := context.WithTimeout(ctx, c.options.MessageTimeout)
	attempts, err := c.handle(msgCtx, message(r), handler)
	cancel()
	c.metrics.duration.WithLabelValues("handle").Observe(time.Since(started).Seconds())
	stage := "handle"
	if err == nil {
		c.metrics.events.WithLabelValues("handle_success").Inc()
		stage = "commit"
		started = time.Now()
		err = c.commit(ctx, r)
		c.metrics.duration.WithLabelValues(stage).Observe(time.Since(started).Seconds())
	} else {
		c.metrics.events.WithLabelValues("handle_failure").Inc()
	}
	if err == nil {
		c.metrics.events.WithLabelValues("commit_success").Inc()
		return nil
	}
	if stage == "commit" {
		c.metrics.events.WithLabelValues("commit_failure").Inc()
	}
	failure := &ConsumeError{Stage: stage, Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Attempts: attempts, Err: err}
	logger.L().Error("Kafka 消息处理失败", zap.String("consumer", c.options.ClientID), zap.String("group", c.options.GroupID), zap.String("topic", r.Topic), zap.Int32("partition", r.Partition), zap.Int64("offset", r.Offset), zap.Int("attempt", attempts), zap.String("stage", stage), zap.Float64("latency_ms", float64(time.Since(started))/float64(time.Millisecond)), zap.Error(failure))
	return failure
}

func (c *Consumer) handle(ctx context.Context, m Message, handler Handler) (int, error) {
	var last error
	for n := 1; n <= c.options.Retry.MaxAttempts; n++ {
		if c.runValues != nil && c.runValues.Err() != nil {
			return n - 1, errors.Join(c.runValues.Err(), last)
		}
		if ctx.Err() != nil {
			return n - 1, errors.Join(context.Cause(ctx), last)
		}
		attempt, cancel := context.WithTimeout(ctx, c.options.AttemptTimeout)
		err := c.invoke(attempt, m, handler)
		if attempt.Err() != nil {
			err = errors.Join(err, attempt.Err())
		}
		cancel()
		if c.runValues != nil && c.runValues.Err() != nil {
			return n, errors.Join(c.runValues.Err(), err)
		}
		if ctx.Err() != nil {
			return n, errors.Join(context.Cause(ctx), err)
		}
		if err == nil {
			return n, nil
		}
		last = err
		var permanent *permanentError
		if errors.As(err, &permanent) || n == c.options.Retry.MaxAttempts {
			return n, err
		}
		timer := time.NewTimer(backoff(c.options.Retry.InitialBackoff, c.options.Retry.MaxBackoff, n))
		select {
		case <-ctx.Done():
			timer.Stop()
			return n, errors.Join(context.Cause(ctx), last)
		case <-timer.C:
		}
		c.metrics.events.WithLabelValues("retry").Inc()
	}
	return c.options.Retry.MaxAttempts, last
}

func (c *Consumer) invoke(ctx context.Context, m Message, handler Handler) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Permanent(c.panicError("handle", p))
		}
	}()
	return handler(ctx, m)
}

func (c *Consumer) commit(ctx context.Context, r *kgo.Record) error {
	commitCtx, cancel := context.WithTimeout(ctx, c.options.CommitTimeout)
	defer cancel()
	select {
	case c.commitGate <- struct{}{}:
		defer func() { <-c.commitGate }()
	case <-commitCtx.Done():
		return context.Cause(commitCtx)
	}
	check := func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if commitCtx.Err() != nil {
			return context.Cause(commitCtx)
		}
		if c.runValues != nil && c.runValues.Err() != nil {
			return c.runValues.Err()
		}
		if c.state >= stopping {
			if c.err != nil {
				return c.err
			}
			return ErrClosed
		}
		if !c.owned[partitionKey{r.Topic, r.Partition}] {
			return errors.New("kafka: partition ownership unavailable")
		}
		return nil
	}
	// 步骤1：提交槽等待后和 SDK 实际生成提交请求前都验证状态与归属。
	if err := check(); err != nil {
		return err
	}
	guarded := kgo.PreCommitFnContext(commitCtx, func(_ *kmsg.OffsetCommitRequest) error { return check() })
	return c.client.CommitRecords(guarded, r)
}
