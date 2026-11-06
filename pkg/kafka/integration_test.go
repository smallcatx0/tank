package kafka

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// integrationBroker 只接入显式指定的测试集群，创建独立主题并在结束时删除。
func integrationBroker(t *testing.T, partitions int32) (*kgo.Client, Options) {
	t.Helper()
	brokers := os.Getenv("KAFKA_TEST_BROKERS")
	if brokers == "" {
		t.Skip("设置 KAFKA_TEST_BROKERS 后运行真实 Broker 验证")
	}
	topic := fmt.Sprintf("gtank-d22-%d", time.Now().UnixNano())
	cl, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...), kgo.RecordPartitioner(kgo.ManualPartitioner()), kgo.DialTimeout(3*time.Second), kgo.RequestTimeoutOverhead(3*time.Second), kgo.RecordDeliveryTimeout(10*time.Second))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request := kmsg.NewPtrCreateTopicsRequest()
	request.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: topic, NumPartitions: partitions, ReplicationFactor: 1}}
	request.TimeoutMillis = 10000
	response, err := request.RequestWith(ctx, cl)
	require.NoError(t, err)
	require.Zero(t, response.Topics[0].ErrorCode)
	o := DefaultOptions()
	o.Brokers = strings.Split(brokers, ",")
	o.Topics = []string{topic}
	o.GroupID = topic
	o.StartOffset = "earliest"
	o.AttemptTimeout = time.Second
	o.MessageTimeout = 2 * time.Second
	o.CommitTimeout = 2 * time.Second
	o.Advanced.MaxPollRecords = 4
	o.MaxConcurrency = 2
	o.Advanced.PollTimeout = 100 * time.Millisecond
	o.Advanced.Group = GroupOptions{6 * time.Second, time.Second, 30 * time.Second}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		del := kmsg.NewPtrDeleteTopicsRequest()
		del.TopicNames = []string{topic}
		del.Topics = []kmsg.DeleteTopicsRequestTopic{{Topic: &topic}}
		del.TimeoutMillis = 5000
		_, err := del.RequestWith(ctx, cl)
		if err != nil {
			t.Logf("cleanup topic %s: %v", topic, err)
		}
		groups := kmsg.NewPtrDeleteGroupsRequest()
		groups.Groups = []string{topic}
		_, err = groups.RequestWith(ctx, cl)
		if err != nil {
			t.Logf("cleanup group: %v", err)
		}
		cl.Close()
	})
	return cl, o
}

func produce(t *testing.T, cl *kgo.Client, topic string, partition int32, values ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	records := make([]*kgo.Record, len(values))
	for i, v := range values {
		records[i] = &kgo.Record{Topic: topic, Partition: partition, Value: []byte(v)}
	}
	require.NoError(t, cl.ProduceSync(ctx, records...).FirstErr())
}
func realConsumer(t *testing.T, o Options) *Consumer {
	t.Helper()
	c, err := NewConsumer(o)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Shutdown(ctx)
	})
	return c
}
func ownedCount(c *Consumer) int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.owned) }
func stopReal(t *testing.T, c *Consumer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c.Shutdown(ctx))
}

func TestIntegrationRebalanceAndCommittedResume(t *testing.T) {
	cl, o := integrationBroker(t, 2)
	c1 := realConsumer(t, o)
	var handled atomic.Int32
	handler := func(context.Context, Message) error { handled.Add(1); return nil }
	d1 := asyncTest(t, func() error { return c1.Run(context.Background(), handler) })
	require.Eventually(t, func() bool { return ownedCount(c1) == 2 }, 20*time.Second, 20*time.Millisecond)
	o.ClientID = "gtank-second"
	c2 := realConsumer(t, o)
	d2 := asyncTest(t, func() error { return c2.Run(context.Background(), handler) })
	require.Eventually(t, func() bool { return ownedCount(c1) == 1 && ownedCount(c2) == 1 }, 20*time.Second, 20*time.Millisecond)
	produce(t, cl, o.Topics[0], 0, "a", "b", "c")
	produce(t, cl, o.Topics[0], 1, "d", "e", "f")
	require.Eventually(t, func() bool { return handled.Load() == 6 }, 15*time.Second, 10*time.Millisecond)
	stopReal(t, c2)
	require.NoError(t, receive(t, d2))
	require.Eventually(t, func() bool { return ownedCount(c1) == 2 }, 15*time.Second, 20*time.Millisecond)
	stopReal(t, c1)
	require.NoError(t, receive(t, d1))
	// 已提交位点优先于首次位点设置；重新入组后只能读到新记录。
	o.StartOffset = "earliest"
	c3 := realConsumer(t, o)
	values := make(chan string, 10)
	d3 := asyncTest(t, func() error {
		return c3.Run(context.Background(), func(_ context.Context, m Message) error { values <- string(m.Value); return nil })
	})
	require.Eventually(t, func() bool { return ownedCount(c3) == 2 }, 20*time.Second, 20*time.Millisecond)
	produce(t, cl, o.Topics[0], 0, "new")
	select {
	case value := <-values:
		require.Equal(t, "new", value)
	case <-time.After(15 * time.Second):
		t.Fatal("no resumed message")
	}
	stopReal(t, c3)
	require.NoError(t, receive(t, d3))
	require.Empty(t, values)
}

func TestIntegrationFailureRedelivery(t *testing.T) {
	cl, o := integrationBroker(t, 1)
	produce(t, cl, o.Topics[0], 0, "failed", "next")
	c1 := realConsumer(t, o)
	sentinel := errors.New("reject message")
	d1 := asyncTest(t, func() error {
		return c1.Run(context.Background(), func(context.Context, Message) error { return Permanent(sentinel) })
	})
	select {
	case err := <-d1:
		require.ErrorIs(t, err, sentinel)
	case <-time.After(20 * time.Second):
		t.Fatal("failure not returned")
	}
	c2 := realConsumer(t, o)
	values := make(chan string, 10)
	d2 := asyncTest(t, func() error {
		return c2.Run(context.Background(), func(_ context.Context, m Message) error { values <- string(m.Value); return nil })
	})
	for _, want := range []string{"failed", "next"} {
		select {
		case value := <-values:
			require.Equal(t, want, value)
		case <-time.After(20 * time.Second):
			t.Fatal("no redelivery")
		}
	}
	stopReal(t, c2)
	require.NoError(t, receive(t, d2))
}

func TestIntegrationReadCommitted(t *testing.T) {
	_, o := integrationBroker(t, 1)
	producer, err := kgo.NewClient(kgo.SeedBrokers(o.Brokers...), kgo.TransactionalID(o.GroupID+"-txn"), kgo.RecordPartitioner(kgo.ManualPartitioner()), kgo.TransactionTimeout(30*time.Second), kgo.RecordDeliveryTimeout(10*time.Second))
	require.NoError(t, err)
	defer producer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, producer.BeginTransaction())
	produce(t, producer, o.Topics[0], 0, "aborted")
	require.NoError(t, producer.EndTransaction(ctx, kgo.TryAbort))
	require.NoError(t, producer.BeginTransaction())
	produce(t, producer, o.Topics[0], 0, "committed")
	require.NoError(t, producer.EndTransaction(ctx, kgo.TryCommit))
	c := realConsumer(t, o)
	values := make(chan string, 10)
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(_ context.Context, m Message) error { values <- string(m.Value); return nil })
	})
	select {
	case value := <-values:
		require.Equal(t, "committed", value)
	case <-time.After(20 * time.Second):
		t.Fatal("no committed transaction")
	}
	stopReal(t, c)
	require.NoError(t, receive(t, done))
	require.Empty(t, values)
}

func TestIntegrationDecompressionLimit(t *testing.T) {
	cl, o := integrationBroker(t, 1)
	produce(t, cl, o.Topics[0], 0, strings.Repeat("compressed-payload", 8192))
	o.Advanced.MaxDecompressBatchBytes = 64 << 10
	c := realConsumer(t, o)
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(context.Context, Message) error { return errors.New("oversized payload must not be dispatched") })
	})
	select {
	case err := <-done:
		require.Error(t, err)
		var tooLarge *kgo.ErrDecompressTooLarge
		require.ErrorAs(t, err, &tooLarge)
	case <-time.After(20 * time.Second):
		t.Fatal("decompression limit not enforced")
	}
}

func TestIntegrationLatestStartsAfterExistingRecords(t *testing.T) {
	cl, o := integrationBroker(t, 1)
	produce(t, cl, o.Topics[0], 0, "existing")
	o.StartOffset = "latest"
	c := realConsumer(t, o)
	values := make(chan string, 10)
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(_ context.Context, m Message) error { values <- string(m.Value); return nil })
	})
	// 持续发送新记录直到消费开始，不依赖 SDK 未公开的初始 offset 就绪状态。
	require.Eventually(t, func() bool {
		produce(t, cl, o.Topics[0], 0, "new")
		select {
		case value := <-values:
			require.Equal(t, "new", value)
			return true
		default:
			return false
		}
	}, 20*time.Second, 100*time.Millisecond)
	stopReal(t, c)
	require.NoError(t, receive(t, done))
	close(values)
	for value := range values {
		require.Equal(t, "new", value)
	}
}
