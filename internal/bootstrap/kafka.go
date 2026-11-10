package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"serv_tpl/internal/global"
	"serv_tpl/internal/telemetry"
	"serv_tpl/pkg/kafka"
)

// kafkaConsumer 是启动装配所需的最小消费契约；生产环境使用 *kafka.Consumer，
// 测试中注入替身，独立验证运行失败、排空与关闭超时，无需连接真实 Kafka。
type kafkaConsumer interface {
	Run(context.Context, kafka.Handler) error
	Shutdown(context.Context) error
}

// KafkaService 将消费结果与关闭接入进程生命周期；nil 表示未启用。
type KafkaService struct {
	stopping atomic.Bool
	consumer kafkaConsumer
	done     chan error
	finished chan struct{}
	result   error
	drain    time.Duration
}

// MustKafkaWorker 构造并启动消费者；构造成功不代表已连接 Broker。
// observation 必须非 nil；关闭观测时传入关闭态 Runtime。
func MustKafkaWorker(cfg global.KafkaConfig, handler kafka.Handler, observation *telemetry.Runtime) *KafkaService {
	if !cfg.Enabled {
		return nil
	}
	if handler == nil {
		panic("Kafka handler 不能为空")
	}
	consumer, err := kafka.NewConsumer(cfg.Options)
	if err != nil {
		panic(err)
	}
	if observation.Registry != nil {
		if err := observation.Registry.Register(consumer); err != nil {
			warnObservation("kafka_register")
		}
	}
	handler = observation.KafkaHandler(handler)
	global.KafkaCli = consumer
	return startKafka(consumer, handler, cfg.Drain)
}

func startKafka(consumer kafkaConsumer, handler kafka.Handler, drain time.Duration) *KafkaService {
	s := &KafkaService{consumer: consumer, done: make(chan error, 1), finished: make(chan struct{}), drain: drain}
	// 步骤1：运行结果使用缓冲通道，保证退出监听尚未启动时也不会阻塞清理。
	go func() {
		defer close(s.finished)
		defer func() {
			if recover() != nil {
				s.result = errors.New("Kafka Run panic")
				logPanic("kafka")
			}
			if s.stopping.Load() && (s.result == kafka.ErrClosed || s.result == kafka.ErrAlreadyRun) {
				s.result = nil
			}
			s.done <- s.result
			close(s.done)
		}()
		s.result = consumer.Run(context.Background(), handler)
	}()
	return s
}

// Done 返回消费者运行结果；关闭配置时返回 nil。
func (s *KafkaService) Done() <-chan error {
	if s == nil {
		return nil
	}
	return s.done
}

// Shutdown 使用独立期限排空消费者，并等待启动包装协程退出后再允许关闭日志。
func (s *KafkaService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopping.Store(true)
	drain, cancel := context.WithTimeout(ctx, s.drain)
	defer cancel()
	err := s.consumer.Shutdown(drain)
	select {
	case <-s.finished:
		return errors.Join(err, s.result)
	case <-drain.Done():
		return errors.Join(err, drain.Err())
	}
}
