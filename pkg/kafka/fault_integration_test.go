package kafka

import (
	"context"
	"crypto/x509"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kmsg"
)

func TestDockerAuthenticationAndNetworkFaults(t *testing.T) {
	f := startFaultFixture(t)
	t.Run("authentication", func(t *testing.T) { testRealAuthentication(t, f) })
	t.Run("transport_reconnect", func(t *testing.T) { testTransportReconnect(t, f) })
	t.Run("commit_request_lost", func(t *testing.T) { testCommitLoss(t, f, false) })
	t.Run("commit_response_lost", func(t *testing.T) { testCommitLoss(t, f, true) })
	t.Run("heartbeat_session_lost", func(t *testing.T) { testSessionLoss(t, f) })
	t.Run("broker_crash_and_recovery", func(t *testing.T) { testBrokerRecovery(t, f) })
	t.Run("existing_integration_regressions", func(t *testing.T) {
		// 常规回归也使用自有 Broker，不依赖开发者常驻容器是否正在重建。
		t.Setenv("KAFKA_TEST_BROKERS", f.proxy.listener.Addr().String())
		t.Run("rebalance", TestIntegrationRebalanceAndCommittedResume)
		t.Run("redelivery", TestIntegrationFailureRedelivery)
		t.Run("transactions", TestIntegrationReadCommitted)
		t.Run("decompression", TestIntegrationDecompressionLimit)
		t.Run("latest", TestIntegrationLatestStartsAfterExistingRecords)
	})
}

func testTransportReconnect(t *testing.T, f *faultFixture) {
	o := f.options(t)
	c := realConsumer(t, o)
	var calls atomic.Int32
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(context.Context, Message) error { calls.Add(1); return nil })
	})
	produce(t, f.admin, o.Topics[0], 0, "before-disconnect")
	require.Eventually(t, func() bool { return calls.Load() == 1 }, 10*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return committedOffset(t, f.admin, o) == 1 }, 5*time.Second, 20*time.Millisecond)
	f.proxy.disconnectAll()
	produce(t, f.admin, o.Topics[0], 0, "after-reconnect")
	require.Eventually(t, func() bool { return calls.Load() == 2 }, 10*time.Second, 20*time.Millisecond)
	stopReal(t, c)
	require.NoError(t, awaitRun(t, done, time.Second))
	require.EqualValues(t, 2, committedOffset(t, f.admin, o))
	t.Log("真实 TCP 连接被关闭后，同一 Consumer 自动重连继续消费，Run 未被普通断连误终止")
}

func testRealAuthentication(t *testing.T, f *faultFixture) {
	for _, mechanism := range []string{"plain", "scram-sha-256", "scram-sha-512"} {
		t.Run(mechanism, func(t *testing.T) {
			o := f.options(t)
			o.Brokers = []string{f.authAddress}
			o.TLS = f.tls.Clone()
			o.SASL = SASLOptions{mechanism, "d22user", f.password}
			produce(t, f.admin, o.Topics[0], 0, "authenticated")
			c := realConsumer(t, o)
			var calls atomic.Int32
			done := asyncTest(t, func() error {
				return c.Run(context.Background(), func(context.Context, Message) error { calls.Add(1); return nil })
			})
			require.Eventually(t, func() bool { return calls.Load() == 1 }, 10*time.Second, 20*time.Millisecond)
			stopReal(t, c)
			require.NoError(t, awaitRun(t, done, time.Second))
			require.EqualValues(t, 1, committedOffset(t, f.admin, o))
			// 错误凭据必须由真实 Broker 拒绝，且 Run 不能无限重试认证。
			o.SASL.Password = "invalid-test-password"
			bad := realConsumer(t, o)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := bad.Run(ctx, func(context.Context, Message) error { t.Error("认证失败仍派发消息"); return nil })
			require.ErrorIs(t, err, kerr.SaslAuthenticationFailed)
			require.NotContains(t, err.Error(), f.password)
			t.Log("真实 TLS 消费与提交成功；错误 SASL 凭据被拒绝且 Run 返回认证错误")
		})
	}
	for _, kind := range []string{"untrusted_ca", "hostname_mismatch"} {
		t.Run(kind, func(t *testing.T) {
			o := f.options(t)
			o.Brokers = []string{f.authAddress}
			o.TLS = f.tls.Clone()
			o.SASL = SASLOptions{"plain", "d22user", f.password}
			if kind == "untrusted_ca" {
				o.TLS.RootCAs = x509.NewCertPool()
			} else {
				o.TLS.ServerName = "wrong.example.invalid"
			}
			c := realConsumer(t, o)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			err := c.Run(ctx, func(context.Context, Message) error { t.Error("TLS 校验失败仍派发消息"); return nil })
			if kind == "untrusted_ca" {
				var unknown x509.UnknownAuthorityError
				require.ErrorAs(t, err, &unknown)
			} else {
				var hostname x509.HostnameError
				require.ErrorAs(t, err, &hostname)
			}
		})
	}
	t.Run("authorization_denied", func(t *testing.T) {
		o := f.options(t)
		o.Brokers = []string{f.authAddress}
		o.TLS = f.tls.Clone()
		o.SASL = SASLOptions{"plain", "denied", f.password}
		c := realConsumer(t, o)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := c.Run(ctx, func(context.Context, Message) error { t.Error("无权限用户仍派发消息"); return nil })
		require.True(t, errors.Is(err, kerr.TopicAuthorizationFailed) || errors.Is(err, kerr.GroupAuthorizationFailed), "unexpected error: %v", err)
	})
}

func testCommitLoss(t *testing.T, f *faultFixture, loseResponse bool) {
	o := f.options(t)
	produce(t, f.admin, o.Topics[0], 0, "first", "second")
	beforeRequests, beforeResponses := f.proxy.commitRequests.Load(), f.proxy.commitResponses.Load()
	if loseResponse {
		f.proxy.dropCommitResponse.Store(true)
	} else {
		f.proxy.dropCommitRequest.Store(true)
	}
	defer f.proxy.dropCommitRequest.Store(false)
	defer f.proxy.dropCommitResponse.Store(false)
	c := realConsumer(t, o)
	var calls atomic.Int32
	started := time.Now()
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(context.Context, Message) error { calls.Add(1); return nil })
	})
	err := awaitRun(t, done, 12*time.Second)
	var failure *ConsumeError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "commit", failure.Stage)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 1, calls.Load())
	require.Greater(t, f.proxy.commitRequests.Load(), beforeRequests)
	wantOffset := int64(-1)
	wantValue := "first"
	if loseResponse {
		wantOffset = 1
		wantValue = "second"
		require.Greater(t, f.proxy.commitResponses.Load(), beforeResponses)
	}
	require.Equal(t, wantOffset, committedOffset(t, f.admin, o))
	f.proxy.dropCommitRequest.Store(false)
	f.proxy.dropCommitResponse.Store(false)
	// 新实例以服务端实际位点恢复，证明 ACK 丢失时结果不确定但不能重跑当前 Handler。
	o.ClientID = "d22-recovery"
	next := realConsumer(t, o)
	seen := make(chan string, 1)
	stop := errors.New("inspection complete")
	nextDone := asyncTest(t, func() error {
		return next.Run(context.Background(), func(_ context.Context, m Message) error { seen <- string(m.Value); return Permanent(stop) })
	})
	require.ErrorIs(t, awaitRun(t, nextDone, 10*time.Second), stop)
	require.Equal(t, wantValue, <-seen)
	t.Logf("网络故障已命中；Run %.2fs 终止，Handler=1，Broker offset=%d，重启读取=%s", time.Since(started).Seconds(), wantOffset, wantValue)
}

func groupEmpty(t *testing.T, f *faultFixture, group string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := kmsg.NewPtrDescribeGroupsRequest()
	request.Groups = []string{group}
	response, err := request.RequestWith(ctx, f.admin)
	return err == nil && len(response.Groups) == 1 && response.Groups[0].ErrorCode == 0 && len(response.Groups[0].Members) == 0
}

func testSessionLoss(t *testing.T, f *faultFixture) {
	o := f.options(t)
	o.AttemptTimeout = 10 * time.Second
	o.MessageTimeout = 10 * time.Second
	o.Advanced.Group.RebalanceTimeout = 20 * time.Second
	produce(t, f.admin, o.Topics[0], 0, "owned-before-loss", "must-not-dispatch")
	c := realConsumer(t, o)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(ctx context.Context, _ Message) error {
			calls.Add(1)
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	})
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("handler not entered")
	}
	before := f.proxy.heartbeats.Load()
	f.proxy.dropHeartbeat.Store(true)
	defer f.proxy.dropHeartbeat.Store(false)
	require.Eventually(t, func() bool { return f.proxy.heartbeats.Load() > before && groupEmpty(t, f, o.GroupID) }, 8*time.Second, 100*time.Millisecond)
	close(release)
	err := awaitRun(t, done, 10*time.Second)
	require.Error(t, err)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 1, calls.Load())
	require.EqualValues(t, -1, committedOffset(t, f.admin, o))
	require.True(t, errors.Is(err, kerr.UnknownMemberID) || errors.Is(err, kerr.IllegalGeneration) || isGroupFailure(err), "unexpected ownership loss: %v", err)
	t.Logf("真实 Broker 已移除失联成员；未提交失去所有权后的消息，Run=%v", err)
}

func isGroupFailure(err error) bool {
	var failure *ConsumeError
	return errors.As(err, &failure) && failure.Stage == "group"
}

func testBrokerRecovery(t *testing.T, f *faultFixture) {
	o := f.options(t)
	produce(t, f.admin, o.Topics[0], 0, "survive-crash", "next")
	c := realConsumer(t, o)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	done := asyncTest(t, func() error {
		return c.Run(context.Background(), func(ctx context.Context, _ Message) error {
			calls.Add(1)
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	})
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("handler not entered")
	}
	// 强制停止真实 Broker，保留容器日志目录；只操作本测试创建的容器。
	dockerTestCommand(t, "kill", f.name)
	defer func() { dockerTestCommand(t, "start", f.name); f.ready(t) }()
	close(release)
	err := awaitRun(t, done, 12*time.Second)
	require.Error(t, err)
	require.EqualValues(t, 1, calls.Load())
	dockerTestCommand(t, "start", f.name)
	f.ready(t)
	require.EqualValues(t, -1, committedOffset(t, f.admin, o))
	o.ClientID = "d22-after-restart"
	next := realConsumer(t, o)
	seen := make(chan string, 2)
	nextDone := asyncTest(t, func() error {
		return next.Run(context.Background(), func(_ context.Context, m Message) error { seen <- string(m.Value); return nil })
	})
	for _, want := range []string{"survive-crash", "next"} {
		select {
		case got := <-seen:
			require.Equal(t, want, got)
		case <-time.After(15 * time.Second):
			t.Fatal("Broker 恢复后未继续消费")
		}
	}
	stopReal(t, next)
	require.NoError(t, awaitRun(t, nextDone, time.Second))
	require.EqualValues(t, 2, committedOffset(t, f.admin, o))
	t.Log("Broker 强制终止后提交失败；重启后日志保留，未提交记录重投并成功提交 offset=2")
}
