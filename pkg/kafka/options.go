// Package kafka 提供逐条同步提交、有限重试和可排空关闭的 Kafka 消费组件。
package kafka

import (
	"crypto/tls"
	"fmt"
	"math"
	"strings"
	"time"
)

// Options 是消费者配置；应从 DefaultOptions 构造，再填写必填字段。
type Options struct {
	// Brokers 是种子节点地址，至少一个非空值。
	Brokers []string
	// GroupID 是消费者组标识。
	GroupID string
	// Topics 是不重复的明确主题名，不支持正则。
	Topics []string
	// ClientID 用于日志、指标和 Kafka 客户端定位。
	ClientID string
	// StartOffset 必须为 earliest/latest，仅用于没有有效提交位点时。
	StartOffset string
	// MaxConcurrency 限制跨分区并发，同一分区始终串行。
	MaxConcurrency int
	// AttemptTimeout 是单次回调的协作取消期限。
	AttemptTimeout time.Duration
	// MessageTimeout 包含全部业务尝试与退避，不包含提交。
	MessageTimeout time.Duration
	// CommitTimeout 包含等待提交入口及 SDK 提交尝试。
	CommitTimeout time.Duration
	// Retry 配置业务重试，不影响 SDK 请求重试。
	Retry RetryOptions
	// TLS 在构造时 Clone；内部证书、回调等引用对象随后必须保持只读。
	TLS *tls.Config
	// SASL 指定认证机制，凭据不得进入日志。
	SASL SASLOptions
	// Advanced 配置传输、拉取与消费者组预算。
	Advanced AdvancedOptions
}

// RetryOptions 限制业务尝试总次数及指数退避。
type RetryOptions struct {
	// MaxAttempts 是包含首次调用的总次数。
	MaxAttempts int
	// InitialBackoff 是第一次重试的退避基准。
	InitialBackoff time.Duration
	// MaxBackoff 限制指数退避上限。
	MaxBackoff time.Duration
}

// SASLOptions 指定认证机制与凭据；启用认证必须同时使用 TLS。
type SASLOptions struct {
	// Mechanism 支持空值/none、plain、scram-sha-256、scram-sha-512。
	Mechanism string
	// Username 是认证用户名。
	Username string
	// Password 是认证密码，不会出现在组件构造的错误中。
	Password string
}

// GroupOptions 限定经典消费者组的心跳与再均衡期限。
type GroupOptions struct {
	// SessionTimeout 是心跳失联期限，须位于 Broker 允许范围内。
	SessionTimeout time.Duration
	// HeartbeatInterval 至多为 SessionTimeout 的三分之一。
	HeartbeatInterval time.Duration
	// RebalanceTimeout 必须大于最坏轮次预算加 5 秒协调余量。
	RebalanceTimeout time.Duration
}

// AdvancedOptions 配置传输、拉取与解压保护；不代表进程内存硬上限。
type AdvancedOptions struct {
	// DialTimeout 限制每次连接建立。
	DialTimeout time.Duration
	// RequestTimeoutOverhead 是协议等待之外的附加请求期限。
	RequestTimeoutOverhead time.Duration
	// RequestRetryTimeout 是普通请求重试总预算，不用于 fetch。
	RequestRetryTimeout time.Duration
	// RequestMaxAttempts 是可重试请求的总尝试上限。
	RequestMaxAttempts int
	// PollTimeout 限制一次空闲拉取等待。
	PollTimeout time.Duration
	// MaxPollRecords 是每轮所有分区合计的应用层记录上限。
	MaxPollRecords int
	// FetchMinBytes 是 Broker 期望累计的最少响应字节数。
	FetchMinBytes int32
	// FetchMaxBytes 是单次 Broker 拉取目标上限。
	FetchMaxBytes int32
	// FetchMaxPartitionBytes 是单分区拉取目标上限。
	FetchMaxPartitionBytes int32
	// FetchMaxWait 限制 Broker 等待最低字节数的时间。
	FetchMaxWait time.Duration
	// MaxConcurrentFetches 限制底层并发 fetch 数。
	MaxConcurrentFetches int
	// BrokerMaxReadBytes 限制协议响应大小。
	BrokerMaxReadBytes int32
	// MaxDecompressBatchBytes 限制单批解压大小，默认 64 MiB。
	MaxDecompressBatchBytes int
	// Group 配置 classic 消费者组期限。
	Group GroupOptions
}

// DefaultOptions 返回唯一默认配置，Brokers、GroupID、Topics、StartOffset 仍须填写。
func DefaultOptions() Options {
	return Options{
		ClientID: "gtank-consumer", MaxConcurrency: 1, AttemptTimeout: 10 * time.Second,
		MessageTimeout: 30 * time.Second, CommitTimeout: 5 * time.Second,
		Retry: RetryOptions{3, 200 * time.Millisecond, 2 * time.Second},
		Advanced: AdvancedOptions{
			DialTimeout: 5 * time.Second, RequestTimeoutOverhead: 5 * time.Second, RequestRetryTimeout: 30 * time.Second,
			RequestMaxAttempts: 3, PollTimeout: time.Second, MaxPollRecords: 1,
			FetchMinBytes: 1, FetchMaxBytes: 1 << 20, FetchMaxPartitionBytes: 1 << 20, FetchMaxWait: 500 * time.Millisecond,
			MaxConcurrentFetches: 1, BrokerMaxReadBytes: 16 << 20, MaxDecompressBatchBytes: 64 << 20,
			Group: GroupOptions{45 * time.Second, 3 * time.Second, 60 * time.Second},
		},
	}
}

// Validate 供入口在装配消费者前校验必填项与非零取值，使配置错误在连接存储之前暴露。
func Validate(o Options) error {
	return o.validate()
}

func (o Options) validate() error {
	if len(o.Brokers) == 0 || len(o.Topics) == 0 || strings.TrimSpace(o.GroupID) == "" || strings.TrimSpace(o.ClientID) == "" {
		return fmt.Errorf("kafka: Brokers、Topics、GroupID、ClientID 必须非空")
	}
	for _, b := range o.Brokers {
		if strings.TrimSpace(b) == "" {
			return fmt.Errorf("kafka: Brokers 含空地址")
		}
	}
	seen := make(map[string]bool, len(o.Topics))
	for _, t := range o.Topics {
		if strings.TrimSpace(t) == "" || seen[t] {
			return fmt.Errorf("kafka: Topics 含空值或重复项")
		}
		seen[t] = true
	}
	if o.StartOffset != "earliest" && o.StartOffset != "latest" {
		return fmt.Errorf("kafka: StartOffset 必须为 earliest/latest")
	}
	a := o.Advanced
	for name, v := range map[string]time.Duration{
		"AttemptTimeout": o.AttemptTimeout, "MessageTimeout": o.MessageTimeout, "CommitTimeout": o.CommitTimeout,
		"Retry.InitialBackoff": o.Retry.InitialBackoff, "Retry.MaxBackoff": o.Retry.MaxBackoff,
		"DialTimeout": a.DialTimeout, "RequestTimeoutOverhead": a.RequestTimeoutOverhead, "RequestRetryTimeout": a.RequestRetryTimeout,
		"PollTimeout": a.PollTimeout, "FetchMaxWait": a.FetchMaxWait, "SessionTimeout": a.Group.SessionTimeout,
		"HeartbeatInterval": a.Group.HeartbeatInterval, "RebalanceTimeout": a.Group.RebalanceTimeout,
	} {
		if v <= 0 {
			return fmt.Errorf("kafka: %s 必须为正数", name)
		}
	}
	if o.MaxConcurrency < 1 || o.Retry.MaxAttempts < 1 || a.RequestMaxAttempts < 1 || a.MaxPollRecords < 1 || a.MaxConcurrentFetches < 1 {
		return fmt.Errorf("kafka: 并发、拉取条数及尝试次数必须为正数")
	}
	if o.MessageTimeout < o.AttemptTimeout || o.Retry.MaxBackoff < o.Retry.InitialBackoff || a.Group.HeartbeatInterval > a.Group.SessionTimeout/3 {
		return fmt.Errorf("kafka: 处理、退避或心跳期限关系非法")
	}
	if a.FetchMinBytes < 1 || a.FetchMaxPartitionBytes < 1 || a.FetchMaxBytes < a.FetchMaxPartitionBytes || a.FetchMinBytes > a.FetchMaxBytes || a.BrokerMaxReadBytes <= a.FetchMaxBytes || a.MaxDecompressBatchBytes < 1 || a.MaxDecompressBatchBytes > math.MaxInt32 {
		return fmt.Errorf("kafka: 拉取、读取或解压大小非法")
	}
	// 步骤1：先检查加法和乘法边界，再比较最坏分区串行预算。
	const reserve = 5 * time.Second
	if o.MessageTimeout > time.Duration(math.MaxInt64)-o.CommitTimeout {
		return fmt.Errorf("kafka: 单条预算溢出")
	}
	t := o.MessageTimeout + o.CommitTimeout
	if int64(a.MaxPollRecords) > (math.MaxInt64-int64(reserve))/int64(t) || time.Duration(a.MaxPollRecords)*t+reserve >= a.Group.RebalanceTimeout {
		return fmt.Errorf("kafka: 最坏轮次预算必须小于 RebalanceTimeout")
	}
	if o.TLS != nil && (o.TLS.InsecureSkipVerify || (o.TLS.MinVersion != 0 && o.TLS.MinVersion < tls.VersionTLS12) || (o.TLS.MaxVersion != 0 && o.TLS.MaxVersion < tls.VersionTLS12) || (o.TLS.MaxVersion != 0 && o.TLS.MinVersion > o.TLS.MaxVersion)) {
		return fmt.Errorf("kafka: TLS 必须验证服务端且最低 TLS 1.2")
	}
	switch o.SASL.Mechanism {
	case "", "none":
		if o.SASL.Username != "" || o.SASL.Password != "" {
			return fmt.Errorf("kafka: 未启用 SASL 时不得配置凭据")
		}
	case "plain", "scram-sha-256", "scram-sha-512":
		if o.TLS == nil || o.SASL.Username == "" || o.SASL.Password == "" {
			return fmt.Errorf("kafka: SASL 需要 TLS、用户名及密码")
		}
	default:
		return fmt.Errorf("kafka: SASL 机制不受支持")
	}
	return nil
}
