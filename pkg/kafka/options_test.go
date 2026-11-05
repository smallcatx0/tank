package kafka

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestOptionsValidation(t *testing.T) {
	require.Error(t, DefaultOptions().validate())
	require.NoError(t, validOptions().validate())
	cases := []func(*Options){
		func(o *Options) { o.Brokers = []string{" "} }, func(o *Options) { o.Topics = []string{"t", "t"} }, func(o *Options) { o.StartOffset = "" },
		func(o *Options) { o.MaxConcurrency = 0 }, func(o *Options) { o.Retry.MaxAttempts = 0 }, func(o *Options) { o.Advanced.MaxPollRecords = 0 },
		func(o *Options) { o.Advanced.RequestMaxAttempts = 0 }, func(o *Options) { o.Advanced.MaxConcurrentFetches = 0 },
		func(o *Options) { o.MessageTimeout = o.AttemptTimeout / 2 }, func(o *Options) { o.Retry.MaxBackoff = o.Retry.InitialBackoff / 2 },
		func(o *Options) { o.Advanced.Group.HeartbeatInterval = o.Advanced.Group.SessionTimeout },
		func(o *Options) { o.Advanced.FetchMinBytes = 0 }, func(o *Options) { o.Advanced.FetchMaxBytes = 1 },
		func(o *Options) { o.Advanced.FetchMaxPartitionBytes = 0 }, func(o *Options) { o.Advanced.BrokerMaxReadBytes = 1 },
		func(o *Options) { o.Advanced.MaxDecompressBatchBytes = 0 }, func(o *Options) { o.Advanced.MaxDecompressBatchBytes = math.MaxInt32 + 1 },
		func(o *Options) { o.MessageTimeout = time.Duration(math.MaxInt64) }, func(o *Options) { o.Advanced.MaxPollRecords = math.MaxInt },
		func(o *Options) { o.Advanced.Group.RebalanceTimeout = time.Second },
		func(o *Options) { o.TLS = &tls.Config{InsecureSkipVerify: true} }, func(o *Options) { o.TLS = &tls.Config{MinVersion: tls.VersionTLS11} },
		func(o *Options) { o.TLS = &tls.Config{MaxVersion: tls.VersionTLS11} }, func(o *Options) { o.TLS = &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS12} },
		func(o *Options) { o.SASL = SASLOptions{Mechanism: "unknown"} }, func(o *Options) { o.SASL = SASLOptions{Username: "secret"} },
		func(o *Options) { o.SASL = SASLOptions{Mechanism: "plain", Username: "secret", Password: "secret"} },
	}
	for i, change := range cases {
		o := validOptions()
		change(&o)
		err := o.validate()
		require.Error(t, err, "case %d", i)
		require.NotContains(t, err.Error(), "secret")
	}
	o := validOptions()
	o.Advanced.PollTimeout = 0
	require.Error(t, o.validate())
}

func TestNewConsumerCopiesConfigurationAndMapsOptions(t *testing.T) {
	for _, mechanism := range []string{"none", "plain", "scram-sha-256", "scram-sha-512"} {
		t.Run(mechanism, func(t *testing.T) {
			o := validOptions()
			o.StartOffset = "latest"
			o.TLS = &tls.Config{}
			o.SASL.Mechanism = mechanism
			if mechanism != "none" {
				o.SASL.Username = "test"
				o.SASL.Password = "secret"
			}
			c, err := NewConsumer(o)
			require.NoError(t, err)
			defer func() { require.NoError(t, shutdown(t, c)) }()
			o.Brokers[0] = "changed"
			o.Topics[0] = "changed"
			o.TLS.MinVersion = tls.VersionTLS10
			require.Equal(t, "localhost:19092", c.options.Brokers[0])
			require.Equal(t, "t", c.options.Topics[0])
			require.Equal(t, uint16(tls.VersionTLS12), c.options.TLS.MinVersion)
			cl := c.client.(*kgo.Client)
			require.Equal(t, true, cl.OptValue(kgo.DisableAutoCommit))
			require.Equal(t, true, cl.OptValue(kgo.BlockRebalanceOnPoll))
			require.Equal(t, 64<<20, cl.OptValue(kgo.MaxDecompressBatchBytes))
			require.LessOrEqual(t, cl.OptValue(kgo.RetryBackoffFn).(func(int) time.Duration)(1), 2*time.Second)
			retry := cl.OptValue(kgo.RetryTimeoutFn).(func(int16) time.Duration)
			require.Equal(t, c.options.Advanced.Group.RebalanceTimeout, retry(11))
			require.Equal(t, c.options.Advanced.RequestRetryTimeout, retry(3))
		})
	}
	_, err := NewConsumer(DefaultOptions())
	require.Error(t, err)
	o := validOptions()
	o.Advanced.RequestTimeoutOverhead = time.Hour
	_, err = NewConsumer(o)
	require.Error(t, err)
	for _, level := range []kgo.LogLevel{kgo.LogLevelError, kgo.LogLevelWarn, kgo.LogLevelInfo} {
		clientLogger{}.Log(level, "test", "error", errors.New("sdk error"))
	}
	require.Equal(t, kgo.LogLevelInfo, clientLogger{}.Level())
}

func TestCancellationBeforeRunAndDuringRetry(t *testing.T) {
	c, f := fakeConsumer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Run(ctx, func(context.Context, Message) error { return nil }), context.Canceled)
	require.Empty(t, f.offsets())
	c2, _ := fakeConsumer(t)
	c2.options.Retry.InitialBackoff = time.Hour
	c2.options.Retry.MaxBackoff = time.Hour
	ctx2, cancel2 := context.WithCancel(context.Background())
	n, err := c2.handle(ctx2, Message{}, func(context.Context, Message) error { cancel2(); return errors.New("retry") })
	require.Equal(t, 1, n)
	require.ErrorIs(t, err, context.Canceled)
	ctx3, cancel3 := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel3()
	n, err = c2.handle(ctx3, Message{}, func(context.Context, Message) error { return errors.New("retry") })
	require.Equal(t, 1, n)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
