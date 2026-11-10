package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"serv_tpl/internal/bootstrap"
	"serv_tpl/internal/global"
	"serv_tpl/internal/telemetry"
	"serv_tpl/pkg/kafka"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// TestIntegrationProducerConsumer 执行真实 CLI 生产进程并验证 worker 消费、排空及位点恢复。
func TestIntegrationProducerConsumer(t *testing.T) {
	brokers := os.Getenv("KAFKA_TEST_BROKERS")
	if brokers == "" {
		t.Skip("设置 KAFKA_TEST_BROKERS 后连接专用测试 Kafka")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...), kgo.DialTimeout(3*time.Second), kgo.RequestTimeoutOverhead(5*time.Second))
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	probe, stop := context.WithTimeout(ctx, 5*time.Second)
	err = admin.Ping(probe)
	stop()
	require.NoError(t, err, "测试 Broker 必须已就绪")
	topic := fmt.Sprintf("producer-d28-%d", time.Now().UnixNano())
	create := kmsg.NewPtrCreateTopicsRequest()
	create.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}}
	create.TimeoutMillis = 5000
	reply, err := create.RequestWith(ctx, admin)
	require.NoError(t, err)
	require.Len(t, reply.Topics, 1)
	require.Zero(t, reply.Topics[0].ErrorCode)
	// 步骤1：只清理本次主题及组；失败路径也通过 Cleanup 关闭消费者后再删除。
	t.Cleanup(func() {
		cleanup, end := context.WithTimeout(context.Background(), 15*time.Second)
		defer end()
		groups := kmsg.NewPtrDeleteGroupsRequest()
		groups.Groups = []string{topic}
		gr, err := groups.RequestWith(cleanup, admin)
		if assertCleanup(t, err, "delete group") {
			for _, g := range gr.Groups {
				if g.ErrorCode != 0 && g.ErrorCode != 69 {
					t.Errorf("删除测试组失败 code=%d", g.ErrorCode)
				}
			}
		}
		del := kmsg.NewPtrDeleteTopicsRequest()
		del.TopicNames = []string{topic}
		del.Topics = []kmsg.DeleteTopicsRequestTopic{{Topic: &topic}}
		del.TimeoutMillis = 5000
		dr, err := del.RequestWith(cleanup, admin)
		if assertCleanup(t, err, "delete topic") {
			for _, r := range dr.Topics {
				if r.ErrorCode != 0 {
					t.Errorf("删除测试主题失败 code=%d", r.ErrorCode)
				}
			}
		}
	})
	old := global.KafkaCli
	t.Cleanup(func() { global.KafkaCli = old })
	o := kafka.DefaultOptions()
	o.Brokers = strings.Split(brokers, ",")
	o.Topics = []string{topic}
	o.GroupID = topic
	o.StartOffset = "earliest"
	observation, err := telemetry.New(context.Background(), telemetry.Options{})
	require.NoError(t, err)
	cfg := global.KafkaConfig{Enabled: true, Options: o, Drain: 5 * time.Second}
	for _, phase := range []struct {
		value  string
		count  int
		offset int64
	}{{"phase-one", 3, 0}, {"phase-two", 1, 3}} {
		got := make(chan kafka.Message, 10)
		consumer := bootstrap.MustKafkaWorker(cfg, func(ctx context.Context, m kafka.Message) error {
			m.Key = bytes.Clone(m.Key)
			m.Value = bytes.Clone(m.Value)
			select {
			case got <- m:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, observation)
		t.Cleanup(func() {
			closeCtx, end := context.WithTimeout(context.Background(), 6*time.Second)
			defer end()
			if err := consumer.Shutdown(closeCtx); err != nil {
				t.Errorf("清理消费者: %v", err)
			}
		})
		// 步骤2：通过测试子进程运行 main，验证真实参数解析、退出码与发送回执。
		args := []string{"-test.run=^TestProducerCLIHelper$", "--", "-brokers", brokers, "-topic", topic, "-key", "integration-key", "-message", phase.value, "-count", fmt.Sprint(phase.count), "-timeout", "10s"}
		command := exec.CommandContext(ctx, os.Args[0], args...)
		command.Env = append(os.Environ(), "D28_PRODUCER_HELPER=1")
		output, err := command.CombinedOutput()
		require.NoError(t, err, "producer output: %s", output)
		decoder := json.NewDecoder(bytes.NewReader(output))
		for i := 0; i < phase.count; i++ {
			var r receipt
			require.NoError(t, decoder.Decode(&r))
			require.Equal(t, receipt{topic, 0, phase.offset + int64(i)}, r)
			select {
			case m := <-got:
				require.Equal(t, phase.value, string(m.Value))
				require.Equal(t, "integration-key", string(m.Key))
				require.Equal(t, r.Offset, m.Offset)
				require.Equal(t, r.Partition, m.Partition)
				require.Equal(t, r.Topic, m.Topic)
			case err := <-consumer.Done():
				t.Fatalf("消费者提前退出: %v", err)
			case <-ctx.Done():
				t.Fatal("等待消费超时")
			}
		}
		var extra receipt
		require.ErrorIs(t, decoder.Decode(&extra), io.EOF)
		// 步骤3：排空确保最后一条提交完成，再以同组重启验证不重放旧消息。
		require.NoError(t, consumer.Shutdown(ctx))
		t.Logf("phase=%s consumed=%d offsets=%d..%d", phase.value, phase.count, phase.offset, phase.offset+int64(phase.count)-1)
	}
	// 步骤4：持续模式跨越单条 timeout 运行，三个回执后主动取消；同组验证新位点 4..6。
	got := make(chan kafka.Message, 10)
	consumer := bootstrap.MustKafkaWorker(cfg, func(ctx context.Context, m kafka.Message) error {
		m.Value = bytes.Clone(m.Value)
		select {
		case got <- m:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, observation)
	defer func() {
		closeCtx, end := context.WithTimeout(context.Background(), 6*time.Second)
		defer end()
		require.NoError(t, consumer.Shutdown(closeCtx))
	}()
	continuousCtx, stopContinuous := context.WithCancel(ctx)
	defer stopContinuous()
	out := &cancelAfterReceipts{cancel: stopContinuous}
	producer := producerConfig{brokers: strings.Split(brokers, ","), topic: topic, message: "continuous", continuous: true, interval: 600 * time.Millisecond, timeout: time.Second}
	start := time.Now()
	require.ErrorIs(t, produce(continuousCtx, producer, out), context.Canceled)
	require.Greater(t, time.Since(start), time.Second)
	require.Equal(t, 3, out.count)
	for offset := int64(4); offset <= 6; offset++ {
		select {
		case m := <-got:
			require.Equal(t, offset, m.Offset)
			require.Equal(t, "continuous", string(m.Value))
		case <-ctx.Done():
			t.Fatal("持续生产消息消费超时")
		}
	}
	require.NoError(t, consumer.Shutdown(ctx))
	t.Log("continuous: consumed=3 offsets=4..6, cancellation completed")

}

func assertCleanup(t *testing.T, err error, operation string) bool {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", operation, err)
		return false
	}
	return true
}

type cancelAfterReceipts struct {
	cancel context.CancelFunc
	count  int
}

func (w *cancelAfterReceipts) Write(p []byte) (int, error) {
	w.count++
	if w.count == 3 {
		w.cancel()
	}
	return len(p), nil
}
