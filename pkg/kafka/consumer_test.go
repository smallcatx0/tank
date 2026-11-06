package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

type fakeClient struct {
	fetches   chan kgo.Fetches
	commit    func(context.Context, *kgo.Record) error
	closeFn   func()
	commitsMu sync.Mutex
	commits   []int64
	allows    atomic.Int32
	closes    atomic.Int32
}

func (f *fakeClient) PollRecords(ctx context.Context, _ int) kgo.Fetches {
	select {
	case fetch := <-f.fetches:
		return fetch
	case <-ctx.Done():
		return fetchError(ctx.Err())
	}
}
func (f *fakeClient) CommitRecords(ctx context.Context, rs ...*kgo.Record) error {
	if f.commit != nil {
		if err := f.commit(ctx, rs[0]); err != nil {
			return err
		}
	}
	f.commitsMu.Lock()
	defer f.commitsMu.Unlock()
	f.commits = append(f.commits, rs[0].Offset)
	return nil
}
func (f *fakeClient) AllowRebalance() { f.allows.Add(1) }
func (f *fakeClient) CloseAllowingRebalance() {
	f.closes.Add(1)
	if f.closeFn != nil {
		f.closeFn()
	}
}
func (f *fakeClient) offsets() []int64 {
	f.commitsMu.Lock()
	defer f.commitsMu.Unlock()
	return append([]int64(nil), f.commits...)
}
func fetchError(err error) kgo.Fetches {
	return kgo.Fetches{{Topics: []kgo.FetchTopic{{Topic: "t", Partitions: []kgo.FetchPartition{{Partition: 0, Err: err}}}}}}
}
func records(partition int32, offsets ...int64) kgo.FetchPartition {
	p := kgo.FetchPartition{Partition: partition}
	for _, o := range offsets {
		p.Records = append(p.Records, &kgo.Record{Topic: "t", Partition: partition, Offset: o, Value: []byte("value")})
	}
	return p
}
func batch(ps ...kgo.FetchPartition) kgo.Fetches {
	return kgo.Fetches{{Topics: []kgo.FetchTopic{{Topic: "t", Partitions: ps}}}}
}
func validOptions() Options {
	o := DefaultOptions()
	o.Brokers = []string{"localhost:19092"}
	o.Topics = []string{"t"}
	o.GroupID = "test"
	o.StartOffset = "earliest"
	o.AttemptTimeout = 100 * time.Millisecond
	o.MessageTimeout = time.Second
	o.CommitTimeout = time.Second
	o.Retry.InitialBackoff = time.Millisecond
	o.Retry.MaxBackoff = 2 * time.Millisecond
	o.Advanced.PollTimeout = 5 * time.Millisecond
	o.Advanced.MaxPollRecords = 8
	return o
}
func fakeConsumer(t *testing.T) (*Consumer, *fakeClient) {
	t.Helper()
	c := newConsumer(validOptions())
	f := &fakeClient{fetches: make(chan kgo.Fetches, 4)}
	c.client = f
	c.partitions("assigned", map[string][]int32{"t": {0, 1, 2}})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Shutdown(ctx)
	})
	return c, f
}
func asyncTest(t *testing.T, fn func() error) <-chan error {
	t.Helper()
	ch := make(chan error, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				ch <- fmt.Errorf("test goroutine panic: %v", p)
			}
		}()
		ch <- fn()
	}()
	return ch
}
func receive(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("task did not terminate")
		return nil
	}
}
func shutdown(t *testing.T, c *Consumer) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return c.Shutdown(ctx)
}
func waitState(t *testing.T, c *Consumer, want state) {
	t.Helper()
	require.Eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.state == want }, time.Second, time.Millisecond)
}

func TestOrderedRetryAndNoSkippedOffsets(t *testing.T) {
	c, f := fakeConsumer(t)
	f.fetches <- batch(records(0, 4, 8, 12))
	sentinel := errors.New("business failure")
	var seen []int64
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(_ context.Context, m Message) error {
			seen = append(seen, m.Offset)
			if m.Offset == 8 {
				return sentinel
			}
			return nil
		})
	})
	err := receive(t, done)
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, []int64{4, 8, 8, 8}, seen)
	require.Equal(t, []int64{4}, f.offsets())
	var failure *ConsumeError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, 3, failure.Attempts)
	require.Equal(t, "handle", failure.Stage)
	require.EqualValues(t, 1, f.closes.Load())
	require.ErrorIs(t, shutdown(t, c), sentinel)
	require.ErrorIs(t, c.Run(context.Background(), func(context.Context, Message) error { return nil }), ErrClosed)
}

func TestCommitFailureDoesNotRepeatHandler(t *testing.T) {
	c, f := fakeConsumer(t)
	f.fetches <- batch(records(0, 1, 2))
	sentinel := errors.New("commit failed")
	f.commit = func(context.Context, *kgo.Record) error { return sentinel }
	var calls int
	err := c.Run(context.Background(), func(context.Context, Message) error { calls++; return nil })
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, 1, calls)
	require.Empty(t, f.offsets())
	var failure *ConsumeError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "commit", failure.Stage)
}

func TestGracefulDrainDiscardsQueuedRecords(t *testing.T) {
	c, f := fakeConsumer(t)
	f.fetches <- batch(records(0, 1, 2), records(1, 3))
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(context.Context, Message) error { calls.Add(1); close(entered); <-release; return nil })
	})
	<-entered
	require.ErrorIs(t, c.Run(context.Background(), func(context.Context, Message) error { return nil }), ErrAlreadyRun)
	stopped := asyncTest(t, func() error { return shutdown(t, c) })
	waitState(t, c, draining)
	close(release)
	require.NoError(t, receive(t, done))
	require.NoError(t, receive(t, stopped))
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, []int64{1}, f.offsets())
	require.NoError(t, shutdown(t, c))
	require.EqualValues(t, 1, f.closes.Load())
}

func TestShutdownDeadlineWaitsForActualHandlerReturn(t *testing.T) {
	c, f := fakeConsumer(t)
	f.fetches <- batch(records(0, 1))
	entered := make(chan struct{})
	release := make(chan struct{})
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(context.Context, Message) error { close(entered); <-release; return nil })
	})
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.Shutdown(ctx), context.DeadlineExceeded)
	select {
	case <-c.done:
		t.Fatal("closed before handler returned")
	default:
	}
	close(release)
	require.ErrorIs(t, receive(t, done), context.DeadlineExceeded)
	require.Empty(t, f.offsets())
}

func TestConcurrentShutdownEarliestDeadline(t *testing.T) {
	c, f := fakeConsumer(t)
	f.fetches <- batch(records(0, 1))
	entered := make(chan struct{})
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(ctx context.Context, _ Message) error { close(entered); <-ctx.Done(); return ctx.Err() })
	})
	<-entered
	long := asyncTest(t, func() error { return shutdown(t, c) })
	waitState(t, c, draining)
	short, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.Shutdown(short), context.DeadlineExceeded)
	require.ErrorIs(t, receive(t, done), context.DeadlineExceeded)
	require.ErrorIs(t, receive(t, long), context.DeadlineExceeded)
}

func TestRunCancellationPreservesValuesAndDoesNotCommit(t *testing.T) {
	c, f := fakeConsumer(t)
	f.fetches <- batch(records(0, 1))
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "trace"))
	defer cancel()
	done := asyncTest(t, func() error {
		return c.Run(ctx, func(ctx context.Context, _ Message) error {
			if ctx.Value(key{}) != "trace" {
				return errors.New("lost context value")
			}
			cancel()
			<-ctx.Done()
			return nil
		})
	})
	require.ErrorIs(t, receive(t, done), context.Canceled)
	require.Empty(t, f.offsets())
}

func TestPermanentAndPanicNeverRetry(t *testing.T) {
	for _, panicHandler := range []bool{false, true} {
		t.Run(fmt.Sprint(panicHandler), func(t *testing.T) {
			c, f := fakeConsumer(t)
			f.fetches <- batch(records(0, 1))
			calls := 0
			err := c.Run(context.Background(), func(context.Context, Message) error {
				calls++
				if panicHandler {
					panic("secret-value")
				}
				return Permanent(errors.New("permanent"))
			})
			require.Error(t, err)
			require.Equal(t, 1, calls)
			require.NotContains(t, err.Error(), "secret-value")
			require.Empty(t, f.offsets())
		})
	}
	require.Nil(t, Permanent(nil))
	err := Permanent(errors.New("x"))
	require.Same(t, err, Permanent(err))
	require.Contains(t, err.Error(), "x")
}

func TestLateSuccessTimesOutAndRetriesSerially(t *testing.T) {
	c, _ := fakeConsumer(t)
	c.options.AttemptTimeout = time.Millisecond
	calls := 0
	n, err := c.handle(context.Background(), Message{}, func(ctx context.Context, _ Message) error {
		calls++
		if calls == 1 {
			<-ctx.Done()
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = c.handle(ctx, Message{}, func(ctx context.Context, _ Message) error { <-ctx.Done(); return errors.New("late") })
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestParallelPartitionsAndSerialCommits(t *testing.T) {
	c, f := fakeConsumer(t)
	c.options.MaxConcurrency = 2
	f.fetches <- batch(records(0, 1), records(1, 2), records(2, 3))
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var commits atomic.Int32
	var maxCommit atomic.Int32
	f.commit = func(context.Context, *kgo.Record) error {
		n := commits.Add(1)
		if n > maxCommit.Load() {
			maxCommit.Store(n)
		}
		time.Sleep(time.Millisecond)
		commits.Add(-1)
		return nil
	}
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(context.Context, Message) error { entered <- struct{}{}; <-release; return nil })
	})
	<-entered
	<-entered
	select {
	case <-entered:
		t.Fatal("exceeded concurrency")
	default:
	}
	close(release)
	require.Eventually(t, func() bool { return len(f.offsets()) == 3 }, time.Second, time.Millisecond)
	require.NoError(t, shutdown(t, c))
	require.NoError(t, receive(t, done))
	require.EqualValues(t, 1, maxCommit.Load())
}

func TestFailureCancelsOtherPartitions(t *testing.T) {
	c, f := fakeConsumer(t)
	c.options.MaxConcurrency = 2
	f.fetches <- batch(records(0, 1), records(1, 2))
	entered := make(chan struct{})
	sentinel := errors.New("first failure")
	err := c.Run(context.Background(), func(ctx context.Context, m Message) error {
		if m.Partition == 0 {
			<-entered
			return Permanent(sentinel)
		}
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	require.ErrorIs(t, err, sentinel)
	require.Empty(t, f.offsets())
}

func TestFetchErrorAndPollTimeout(t *testing.T) {
	c, f := fakeConsumer(t)
	sentinel := errors.New("fetch fatal")
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(context.Context, Message) error { return errors.New("must not handle") })
	})
	require.Eventually(t, func() bool { return f.allows.Load() > 0 }, time.Second, time.Millisecond)
	fetch := batch(records(0, 1))
	fetch[0].Topics[0].Partitions[0].Err = sentinel
	f.fetches <- fetch
	require.ErrorIs(t, receive(t, done), sentinel)
	require.Empty(t, f.offsets())
}

func TestValidationAndCreatedShutdown(t *testing.T) {
	c, f := fakeConsumer(t)
	//lint:ignore SA1012 验证公开 API 明确拒绝 nil context 的契约。
	require.Error(t, c.Run(nil, func(context.Context, Message) error { return nil }))
	require.Error(t, c.Run(context.Background(), nil))
	//lint:ignore SA1012 验证公开 API 明确拒绝 nil context 的契约。
	require.Error(t, c.Shutdown(nil))
	require.Error(t, c.Shutdown(context.Background()))
	require.NoError(t, shutdown(t, c))
	require.EqualValues(t, 1, f.closes.Load())
}

func TestClosePanicJoinsOriginalFailure(t *testing.T) {
	c, f := fakeConsumer(t)
	sentinel := errors.New("original")
	f.fetches <- fetchError(sentinel)
	f.closeFn = func() { panic("close") }
	err := c.Run(context.Background(), func(context.Context, Message) error { return nil })
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "stage=close")
}

func TestOwnershipAndCommitGateDeadline(t *testing.T) {
	c, _ := fakeConsumer(t)
	c.state = running
	r := &kgo.Record{Topic: "t", Partition: 0}
	c.partitions("revoked", map[string][]int32{"t": {0}})
	require.ErrorContains(t, c.commit(context.Background(), r), "ownership")
	c.commitGate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	require.ErrorIs(t, c.commit(ctx, r), context.DeadlineExceeded)
	<-c.commitGate
	c.partitions("lost", map[string][]int32{"t": {1}})
	require.ErrorContains(t, c.result(), "lost partition")
	c.finish()
}

func TestMetricsAndMessageShape(t *testing.T) {
	c, _ := fakeConsumer(t)
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(c))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, families)
	r := &kgo.Record{Headers: []kgo.RecordHeader{{Key: "a", Value: nil}, {Key: "a", Value: []byte{}}}, Timestamp: time.Now()}
	m := message(r)
	require.Len(t, m.Headers, 2)
	require.Nil(t, m.Headers[0].Value)
	require.NotNil(t, m.Headers[1].Value)
	require.Equal(t, r.Timestamp, m.Timestamp)
	require.Nil(t, message(&kgo.Record{}).Headers)
}

func TestBackoffBounds(t *testing.T) {
	for n := 1; n < 100; n++ {
		v := backoff(time.Millisecond, 10*time.Millisecond, n)
		require.GreaterOrEqual(t, v, 800*time.Microsecond)
		require.LessOrEqual(t, v, 10*time.Millisecond)
	}
	require.LessOrEqual(t, backoff(1<<62, 1<<62+100, 100), time.Duration(1<<62+100))
}

func BenchmarkMessageAndBackoff(b *testing.B) {
	r := &kgo.Record{Headers: []kgo.RecordHeader{{Key: "a", Value: []byte("b")}}}
	b.ReportAllocs()
	for b.Loop() {
		_ = message(r)
		_ = backoff(time.Millisecond, time.Second, 3)
	}
}

func TestCanceledShutdownBeforeRun(t *testing.T) {
	c, f := fakeConsumer(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	require.ErrorIs(t, c.Shutdown(ctx), context.Canceled)
	select {
	case <-c.done:
	case <-time.After(time.Second):
		t.Fatal("created consumer did not close")
	}
	require.EqualValues(t, 1, f.closes.Load())
}

func TestRunCancellationDoesNotWaitForWatcherScheduling(t *testing.T) {
	c, f := fakeConsumer(t)
	ctx, cancel := context.WithCancel(context.Background())
	c.runValues = ctx
	cancel()
	// 不启动 Run 的异步监听，仍必须在重试和提交入口同步识别取消。
	calls := 0
	n, err := c.handle(context.Background(), Message{}, func(context.Context, Message) error { calls++; return nil })
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, n)
	require.Zero(t, calls)
	require.ErrorIs(t, c.commit(context.Background(), &kgo.Record{Topic: "t", Partition: 0}), context.Canceled)
	require.Empty(t, f.offsets())
}

func BenchmarkConsumerRound(b *testing.B) {
	c := newConsumer(validOptions())
	f := &fakeClient{}
	c.client = f
	c.state = running
	c.owned[partitionKey{"t", 0}] = true
	defer c.finish()
	fetches := batch(records(0, 1))
	handler := func(context.Context, Message) error { return nil }
	b.ReportAllocs()
	for b.Loop() {
		if err := c.round(fetches, handler); err != nil {
			b.Fatal(err)
		}
		f.commits = f.commits[:0]
	}
}
