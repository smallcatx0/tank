package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Header 保留消息头顺序、重名字段和 nil 值。
type Header struct {
	Key   string
	Value []byte
}

// Message 是只读的消息视图；回调返回后保留数据需要调用方自行复制。
type Message struct {
	Topic      string
	Partition  int32
	Offset     int64
	Key, Value []byte
	Headers    []Header
	Timestamp  time.Time
}

// Handler 同步处理一条消息；nil 表示业务副作用已经完成。
type Handler func(context.Context, Message) error

var (
	// ErrAlreadyRun 表示实例已经启动，不允许重复或并发运行。
	ErrAlreadyRun = errors.New("kafka: consumer already run")
	// ErrClosed 表示实例已经关闭。
	ErrClosed = errors.New("kafka: consumer closed")
)

// ConsumeError 保留失败阶段、消息定位与原始错误；无消息时 Partition、Offset 为 -1。
type ConsumeError struct {
	Stage, Topic string
	Partition    int32
	Offset       int64
	Attempts     int
	Err          error
}

// Error 返回可用于诊断的错误描述，不包含消息正文或认证配置。
func (e *ConsumeError) Error() string {
	return fmt.Sprintf("kafka: stage=%s topic=%s partition=%d offset=%d attempts=%d: %v", e.Stage, e.Topic, e.Partition, e.Offset, e.Attempts, e.Err)
}

// Unwrap 保留底层错误的 errors.Is/As 语义。
func (e *ConsumeError) Unwrap() error { return e.Err }

type permanentError struct{ error }

func (e *permanentError) Unwrap() error { return e.error }

// Permanent 标记不可重试业务错误；nil 和重复包装保持原语义。
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	var p *permanentError
	if errors.As(err, &p) {
		return err
	}
	return &permanentError{err}
}
