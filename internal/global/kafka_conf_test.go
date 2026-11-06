package global

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLoadKafkaConfig(t *testing.T) {
	c := viper.New()
	c.Set("kafka.drain_timeout", "invalid")
	cfg, err := LoadKafkaConfig(c, time.Second)
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	c.Set("kafka.enable", "invalid")
	_, err = LoadKafkaConfig(c, 45*time.Second)
	require.Error(t, err)
	c.Set("kafka.enable", true)
	for _, v := range []string{"invalid", "0s", "35s", "45s"} {
		c.Set("kafka.drain_timeout", v)
		_, err = LoadKafkaConfig(c, 45*time.Second)
		require.Error(t, err)
	}
	c = viper.New()
	c.Set("kafka.enable", true)
	c.Set("kafka.brokers", []string{"localhost:19092"})
	c.Set("kafka.group_id", "worker")
	c.Set("kafka.topics", []string{"example"})
	c.Set("kafka.start_offset", "earliest")
	cfg, err = LoadKafkaConfig(c, 45*time.Second)
	require.NoError(t, err)
	require.Equal(t, 40*time.Second, cfg.Drain)
	require.Equal(t, "sth-worker", cfg.Options.ClientID)
	require.Equal(t, "worker", cfg.Options.GroupID)
	require.Equal(t, []string{"example"}, cfg.Options.Topics)
	require.Equal(t, 3, cfg.Options.Retry.MaxAttempts)
}
