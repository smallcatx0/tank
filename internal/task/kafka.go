// Package task 提供可删除的 worker 示例任务。
package task

import (
	"context"
	"time"

	"go.uber.org/zap"
	"serv_tpl/pkg/kafka"
	"serv_tpl/pkg/logger"
)

// LogKafkaMessage 仅记录消息位置；返回成功会提交位点，只用于独立示例主题。
func LogKafkaMessage(ctx context.Context, m kafka.Message) error {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return err
	}
	logger.L().Info("worker 收到示例 Kafka 消息",
		zap.String("trace_id", ""), zap.String("topic", m.Topic),
		zap.Int32("partition", m.Partition), zap.Int64("offset", m.Offset),
		zap.Int64("latency_ms", time.Since(start).Milliseconds()))
	return nil
}
