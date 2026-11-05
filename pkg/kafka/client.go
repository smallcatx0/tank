package kafka

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"go.uber.org/zap"
	"serv_tpl/pkg/logger"
)

type client interface {
	PollRecords(context.Context, int) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	AllowRebalance()
	CloseAllowingRebalance()
}

// NewConsumer 校验并复制配置，创建客户端但不保证网络或入组就绪。
func NewConsumer(o Options) (*Consumer, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	o.Brokers = append([]string(nil), o.Brokers...)
	o.Topics = append([]string(nil), o.Topics...)
	if o.TLS != nil {
		o.TLS = o.TLS.Clone()
		if o.TLS.MinVersion == 0 {
			o.TLS.MinVersion = tls.VersionTLS12
		}
	}
	c := newConsumer(o)
	cl, err := kgo.NewClient(c.clientOptions()...)
	if err != nil {
		c.cancel(err)
		c.pollCancel()
		return nil, fmt.Errorf("kafka: 创建客户端: %w", err)
	}
	c.client = cl
	return c, nil
}

func (c *Consumer) clientOptions() []kgo.Opt {
	o := c.options
	a := o.Advanced
	offset := kgo.NewOffset().AtStart()
	if o.StartOffset == "latest" {
		offset = kgo.NewOffset().AtEnd()
	}
	opts := []kgo.Opt{
		kgo.SeedBrokers(o.Brokers...), kgo.ClientID(o.ClientID), kgo.ConsumerGroup(o.GroupID), kgo.ConsumeTopics(o.Topics...),
		kgo.ConsumeResetOffset(offset), kgo.Balancers(kgo.CooperativeStickyBalancer()), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll(),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()), kgo.DialTimeout(a.DialTimeout), kgo.RequestTimeoutOverhead(a.RequestTimeoutOverhead),
		kgo.RequestRetries(a.RequestMaxAttempts), kgo.RetryTimeoutFn(func(key int16) time.Duration {
			if key == 11 || key == 12 || key == 14 {
				return a.Group.RebalanceTimeout
			}
			return a.RequestRetryTimeout
		}),
		kgo.RetryBackoffFn(func(n int) time.Duration { return backoff(250*time.Millisecond, 2*time.Second, n+1) }),
		kgo.FetchMinBytes(a.FetchMinBytes), kgo.FetchMaxBytes(a.FetchMaxBytes), kgo.FetchMaxPartitionBytes(a.FetchMaxPartitionBytes),
		kgo.FetchMaxWait(a.FetchMaxWait), kgo.MaxConcurrentFetches(a.MaxConcurrentFetches), kgo.BrokerMaxReadBytes(a.BrokerMaxReadBytes),
		kgo.MaxDecompressBatchBytes(a.MaxDecompressBatchBytes),
		kgo.SessionTimeout(a.Group.SessionTimeout), kgo.HeartbeatInterval(a.Group.HeartbeatInterval), kgo.RebalanceTimeout(a.Group.RebalanceTimeout),
		kgo.WithLogger(clientLogger{}), kgo.WithHooks(clientErrors{c}),
		kgo.OnPartitionsAssigned(func(_ context.Context, _ *kgo.Client, p map[string][]int32) { c.partitions("assigned", p) }),
		kgo.OnPartitionsRevoked(func(_ context.Context, _ *kgo.Client, p map[string][]int32) { c.partitions("revoked", p) }),
		kgo.OnPartitionsLost(func(_ context.Context, _ *kgo.Client, p map[string][]int32) { c.partitions("lost", p) }),
	}
	if o.TLS != nil {
		opts = append(opts, kgo.DialTLSConfig(o.TLS))
	}
	switch o.SASL.Mechanism {
	case "plain":
		opts = append(opts, kgo.SASL(plain.Auth{User: o.SASL.Username, Pass: o.SASL.Password}.AsMechanism()))
	case "scram-sha-256":
		opts = append(opts, kgo.SASL(scram.Auth{User: o.SASL.Username, Pass: o.SASL.Password}.AsSha256Mechanism()))
	case "scram-sha-512":
		opts = append(opts, kgo.SASL(scram.Auth{User: o.SASL.Username, Pass: o.SASL.Password}.AsSha512Mechanism()))
	}
	return opts
}

func (c *Consumer) partitions(event string, partitions map[string][]int32) {
	defer func() {
		if p := recover(); p != nil {
			c.stop(c.panicError("group", p))
		}
	}()
	c.mu.Lock()
	for topic, ps := range partitions {
		for _, p := range ps {
			key := partitionKey{topic, p}
			if event == "assigned" {
				c.owned[key] = true
			} else {
				delete(c.owned, key)
			}
		}
	}
	c.mu.Unlock()
	c.metrics.events.WithLabelValues("rebalance_" + event).Inc()
	logger.L().Info("Kafka 分区归属变化", zap.String("consumer", c.options.ClientID), zap.String("group", c.options.GroupID), zap.String("event", event), zap.Any("partitions", partitions))
	if event == "lost" {
		c.stop(&ConsumeError{Stage: "group", Partition: -1, Offset: -1, Err: fmt.Errorf("consumer lost partition ownership")})
	}
}

type clientLogger struct{}

func (clientLogger) Level() kgo.LogLevel { return kgo.LogLevelInfo }
func (clientLogger) Log(level kgo.LogLevel, msg string, kv ...any) {
	// SDK 日志不转储任意键值，避免将协议响应或认证内容带入日志。
	fields := []zap.Field{zap.String("component", "kafka.client")}
	for i := 0; i+1 < len(kv); i += 2 {
		if err, ok := kv[i+1].(error); ok {
			fields = append(fields, zap.Error(err))
		}
	}
	switch level {
	case kgo.LogLevelError:
		logger.L().Error(msg, fields...)
	case kgo.LogLevelWarn:
		logger.L().Warn(msg, fields...)
	default:
		logger.L().Info(msg, fields...)
	}
}
