// Command kafka_producer 向明确指定的测试主题发送消息，用于生产消费联调。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"serv_tpl/internal/bootstrap"

	"github.com/subosito/gotenv"
	"github.com/twmb/franz-go/pkg/kgo"
)

type producerConfig struct {
	continuous          bool
	interval            time.Duration
	brokers             []string
	topic, key, message string
	count               int
	timeout             time.Duration
}

type receipt struct {
	Topic     string `json:"topic"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
}

func main() {
	stopLog := bootstrap.MustInitLog()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := execute(ctx, os.Args[1:], os.Stdout, os.Getenv)
	if ctx.Err() == context.Canceled && errors.Is(err, context.Canceled) {
		err = nil
	}
	cancel()
	closeErr := stopLog(context.Background())
	if errors.Is(err, flag.ErrHelp) {
		err = nil
	}
	if err = errors.Join(err, closeErr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	cfg, err := parseConfig(args, out, getenv)
	if err != nil {
		return err
	}
	return produce(ctx, cfg, out)
}

func parseConfig(args []string, out io.Writer, getenv func(string) string) (producerConfig, error) {
	var cfg producerConfig
	fs := flag.NewFlagSet("kafka_producer", flag.ContinueOnError)
	fs.SetOutput(out)
	brokers := fs.String("brokers", "", "Kafka host:port 列表，逗号分隔")
	fs.StringVar(&cfg.topic, "topic", "", "已存在的测试主题")
	fs.StringVar(&cfg.key, "key", "", "消息 key，不记录到输出")
	fs.StringVar(&cfg.message, "message", "hello kafka", "消息正文，不记录到输出")
	fs.IntVar(&cfg.count, "count", 1, "发送数量，1..10000")
	fs.DurationVar(&cfg.timeout, "timeout", 15*time.Second, "有限模式总期限；持续模式单条发送期限")
	fs.BoolVar(&cfg.continuous, "continuous", false, "持续发送直到取消或失败，不能同时指定 count")
	fs.DurationVar(&cfg.interval, "interval", time.Second, "持续模式每次发送成功后的等待间隔，必须大于 0")
	envFile := fs.String("env-file", "", "可选 dotenv 配置文件，仅读取 Kafka 地址和主题")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, errors.New("不接受位置参数")
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if cfg.interval <= 0 {
		return cfg, errors.New("interval 必须大于 0")
	}
	if cfg.continuous && explicit["count"] {
		return cfg, errors.New("continuous 不能同时指定 count")
	}
	// 步骤1：文件只作为默认值来源，不污染进程环境，不输出配置内容。
	values := gotenv.Env{}
	if *envFile != "" {
		var err error
		values, err = gotenv.Read(*envFile)
		if err != nil {
			return cfg, errors.New("无法读取或解析 env-file，请检查文件格式和路径")
		}
	}
	fallback := func(key string) string {
		if value := getenv(key); value != "" {
			return value
		}
		return values[key]
	}
	if !explicit["brokers"] {
		*brokers = fallback("KAFKA_BROKERS")
	}
	if !explicit["topic"] {
		cfg.topic = fallback("KAFKA_TOPIC")
	}
	for _, broker := range strings.Split(*brokers, ",") {
		broker = strings.TrimSpace(broker)
		host, port, err := net.SplitHostPort(broker)
		n, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || host == "" || n < 1 || n > 65535 {
			return cfg, errors.New("brokers 必须为非空 host:port 列表，端口范围 1..65535")
		}
		cfg.brokers = append(cfg.brokers, broker)
	}
	if !validTopic(cfg.topic) {
		return cfg, errors.New("topic 必须为有效的明确主题名，长度 1..249")
	}
	if cfg.count < 1 || cfg.count > 10000 || cfg.timeout <= 0 {
		return cfg, errors.New("count 必须为 1..10000，timeout 必须大于 0")
	}
	if len(cfg.key)+len(cfg.message) > (1<<20)-1024 {
		return cfg, errors.New("key 与正文合计不能超过 1MiB-1024 字节")
	}
	return cfg, nil
}

func validTopic(topic string) bool {
	if len(topic) == 0 || len(topic) > 249 || topic == "." || topic == ".." {
		return false
	}
	for _, c := range topic {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func produce(parent context.Context, cfg producerConfig, out io.Writer) error {
	ctx := parent
	if !cfg.continuous {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, cfg.timeout)
		defer cancel()
	}
	// 步骤2：显式限制传输、投递和缓冲；保留 SDK 默认幂等生产，不开启自动建主题。
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.brokers...), kgo.ClientID("kafka-producer-test"),
		kgo.DialTimeout(3*time.Second), kgo.RequestTimeoutOverhead(5*time.Second),
		kgo.RecordDeliveryTimeout(cfg.timeout), kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.MaxBufferedRecords(1), kgo.ProducerBatchMaxBytes(1<<20),
	)
	if err != nil {
		return fmt.Errorf("创建生产者: %w", err)
	}
	defer cl.Close()
	encoder := json.NewEncoder(out)
	return sendLoop(ctx, cfg, func(sendCtx context.Context) error {
		record := &kgo.Record{Topic: cfg.topic, Key: []byte(cfg.key), Value: []byte(cfg.message)}
		if err := cl.ProduceSync(sendCtx, record).FirstErr(); err != nil {
			return fmt.Errorf("发送消息失败: %w", err)
		}
		// 步骤3：仅在 Broker 确认后输出位点；输出失败不回滚已发送的消息。
		if err := encoder.Encode(receipt{record.Topic, record.Partition, record.Offset}); err != nil {
			return fmt.Errorf("消息已发送，但输出回执失败: %w", err)
		}
		return nil
	})
}

// sendLoop 串行调度发送；回调边界便于独立验证限速、超时与取消。
func sendLoop(ctx context.Context, cfg producerConfig, send func(context.Context) error) error {
	for sent := uint64(0); cfg.continuous || sent < uint64(cfg.count); sent++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		sendCtx := ctx
		cancel := func() {}
		if cfg.continuous {
			sendCtx, cancel = context.WithTimeout(ctx, cfg.timeout)
		}
		err := send(sendCtx)
		cancel()
		if err != nil {
			return err
		}
		if cfg.continuous {
			timer := time.NewTimer(cfg.interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}
