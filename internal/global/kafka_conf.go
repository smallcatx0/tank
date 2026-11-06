package global

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/viper"
	"serv_tpl/pkg/kafka"
)

// KafkaConfig 是 worker 使用的消费配置；首版配置面向本机明文 Kafka。
type KafkaConfig struct {
	Enabled bool
	Options kafka.Options
	Drain   time.Duration
}

// LoadKafkaConfig 从组件默认值构造配置；关闭时不解析消费参数。
func LoadKafkaConfig(c *viper.Viper, shutdown time.Duration) (KafkaConfig, error) {
	o := KafkaConfig{Options: kafka.DefaultOptions(), Drain: 40 * time.Second}
	c.SetDefault("kafka.enable", false)
	enabled, err := strconv.ParseBool(fmt.Sprint(c.Get("kafka.enable")))
	if err != nil {
		return o, fmt.Errorf("kafka.enable 必须为布尔值")
	}
	o.Enabled = enabled
	if !enabled {
		return o, nil
	}
	c.SetDefault("kafka.client_id", "sth-worker")
	c.SetDefault("kafka.drain_timeout", "40s")
	o.Options.Brokers = c.GetStringSlice("kafka.brokers")
	o.Options.GroupID = c.GetString("kafka.group_id")
	o.Options.Topics = c.GetStringSlice("kafka.topics")
	o.Options.StartOffset = c.GetString("kafka.start_offset")
	o.Options.ClientID = c.GetString("kafka.client_id")
	o.Drain, err = time.ParseDuration(fmt.Sprint(c.Get("kafka.drain_timeout")))
	if err != nil || o.Drain <= o.Options.MessageTimeout+o.Options.CommitTimeout || o.Drain >= shutdown {
		return o, fmt.Errorf("kafka.drain_timeout 必须大于 35s 且小于 shutdown_timeout")
	}
	return o, nil
}
