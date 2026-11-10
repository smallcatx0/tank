package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"serv_tpl/internal/global"
	"serv_tpl/internal/requestctx"
	"serv_tpl/internal/task"
	"serv_tpl/internal/telemetry"
	"serv_tpl/pkg/kafka"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// TestKafkaWorkerIntegration 使用独立临时主题验证真实消费、排空与提交后的恢复。
func TestKafkaWorkerIntegration(t *testing.T) {
	brokers := os.Getenv("KAFKA_TEST_BROKERS")
	if brokers == "" {
		t.Skip("需显式指定测试 Kafka")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl, err := kgo.NewClient(kgo.SeedBrokers(strings.Split(brokers, ",")...), kgo.DialTimeout(time.Second), kgo.RequestTimeoutOverhead(3*time.Second), kgo.RecordDeliveryTimeout(5*time.Second))
	require.NoError(t, err)
	defer cl.Close()
	topic := fmt.Sprintf("worker-d26-%d", time.Now().UnixNano())
	req := kmsg.NewPtrCreateTopicsRequest()
	req.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}}
	req.TimeoutMillis = 5000
	resp, err := req.RequestWith(ctx, cl)
	require.NoError(t, err)
	require.Zero(t, resp.Topics[0].ErrorCode)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		del := kmsg.NewPtrDeleteTopicsRequest()
		del.TopicNames = []string{topic}
		del.Topics = []kmsg.DeleteTopicsRequestTopic{{Topic: &topic}}
		del.TimeoutMillis = 5000
		// 清理失败只标记测试失败，仍继续清理其他资源。
		deleted, err := del.RequestWith(cleanup, cl)
		if err != nil {
			t.Errorf("删除测试主题 %s: %v", topic, err)
		} else {
			if len(deleted.Topics) != 1 {
				t.Errorf("删除测试主题 %s: 响应数量=%d，期望 1", topic, len(deleted.Topics))
			}
			for _, result := range deleted.Topics {
				if result.ErrorCode != 0 {
					t.Errorf("删除测试主题 %s: code=%d err=%v", topic, result.ErrorCode, kerr.ErrorForCode(result.ErrorCode))
				}
			}
		}
		groups := kmsg.NewPtrDeleteGroupsRequest()
		groups.Groups = []string{topic}
		deletedGroups, err := groups.RequestWith(cleanup, cl)
		if err != nil {
			t.Errorf("删除测试消费者组 %s: %v", topic, err)
		} else {
			if len(deletedGroups.Groups) != 1 {
				t.Errorf("删除测试消费者组 %s: 响应数量=%d，期望 1", topic, len(deletedGroups.Groups))
			}
			for _, result := range deletedGroups.Groups {
				// 测试可能在消费者入组前失败，此时组不存在即无需清理。
				if result.ErrorCode != 0 && result.ErrorCode != kerr.GroupIDNotFound.Code {
					t.Errorf("删除测试消费者组 %s: code=%d err=%v", result.Group, result.ErrorCode, kerr.ErrorForCode(result.ErrorCode))
				}
			}
		}
	}()
	o := kafka.DefaultOptions()
	o.Brokers = strings.Split(brokers, ",")
	o.GroupID = topic
	o.Topics = []string{topic}
	o.StartOffset = "earliest"
	cfg := global.KafkaConfig{Enabled: true, Options: o, Drain: 5 * time.Second}
	for _, value := range []string{"first", "second"} {
		observation, e := telemetry.New(ctx, telemetry.Options{Metrics: telemetry.MetricsOptions{Enable: true}})
		require.NoError(t, e)
		exporter := tracetest.NewInMemoryExporter()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
		defer provider.Shutdown(context.Background())
		observation.Provider = provider
		observation.Options.Tracing.Enable = true
		producerCtx, end := observation.StartProducer(ctx, topic)
		record := &kgo.Record{Topic: topic, Value: []byte(value)}
		for _, h := range observation.InjectKafka(producerCtx, nil) {
			record.Headers = append(record.Headers, kgo.RecordHeader{Key: h.Key, Value: h.Value})
		}
		produceErr := cl.ProduceSync(producerCtx, record).FirstErr()
		end(produceErr)
		require.NoError(t, produceErr)
		got := make(chan string, 2)
		s := MustKafkaWorker(cfg, func(ctx context.Context, m kafka.Message) error {
			if err := task.LogKafkaMessage(ctx, m); err != nil {
				return err
			}
			if requestctx.TraceID(ctx) != requestctx.TraceID(producerCtx) || requestctx.SpanID(ctx) == "" {
				return fmt.Errorf("消费链路未关联生产者")
			}
			got <- string(m.Value)
			return nil
		}, observation)
		defer func() {
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_ = s.Shutdown(cleanup)
		}()
		select {
		case actual := <-got:
			require.Equal(t, value, actual, "重启同一组不得重放已提交消息")
		case err := <-s.Done():
			t.Fatalf("消费提前退出: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		require.NoError(t, s.Shutdown(ctx))
		require.Len(t, exporter.GetSpans(), 2)
		families, e := observation.Registry.Gather()
		require.NoError(t, e)
		found := false
		for _, f := range families {
			if strings.HasPrefix(f.GetName(), "kafka_consumer_") {
				found = true
			}
		}
		require.True(t, found)
	}
	global.KafkaCli = nil
}
