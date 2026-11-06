package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestConnectionFailureBeforeRunPreservesCauseAndCleanup(t *testing.T) {
	c, f := fakeConsumer(t)
	clientErrors{c}.OnBrokerConnect(kgo.BrokerMetadata{}, 0, nil, kerr.SaslAuthenticationFailed)
	// 客户端可能在 Run 前异步连接，错误不能被误报为 AlreadyRun 或漏掉关闭。
	require.Equal(t, created, c.state)
	err := c.Run(context.Background(), func(context.Context, Message) error { t.Error("fatal connection dispatched a message"); return nil })
	require.ErrorIs(t, err, kerr.SaslAuthenticationFailed)
	require.EqualValues(t, 1, f.closes.Load())
}

func TestConnectionFailureBeforeShutdownClosesOnce(t *testing.T) {
	c, f := fakeConsumer(t)
	clientErrors{c}.OnBrokerConnect(kgo.BrokerMetadata{}, 0, nil, kerr.SaslAuthenticationFailed)
	require.ErrorIs(t, shutdown(t, c), kerr.SaslAuthenticationFailed)
	require.EqualValues(t, 1, f.closes.Load())
}

func TestConnectionHookIgnoresTransientFailures(t *testing.T) {
	c, _ := fakeConsumer(t)
	for _, err := range []error{nil, io.EOF, context.DeadlineExceeded, context.Canceled} {
		clientErrors{c}.OnBrokerConnect(kgo.BrokerMetadata{}, 0, nil, err)
	}
	require.NoError(t, c.result())
	require.NoError(t, c.ctx.Err())
}

func TestFatalAuthenticationClassification(t *testing.T) {
	for _, err := range []error{kerr.SaslAuthenticationFailed, kerr.UnsupportedSaslMechanism, kerr.IllegalSaslState,
		&tls.CertificateVerificationError{Err: errors.New("certificate rejected")},
		x509.UnknownAuthorityError{}, x509.HostnameError{}, x509.CertificateInvalidError{},
	} {
		require.True(t, fatalAuthentication(err), "type %T", err)
	}
	require.False(t, fatalAuthentication(errors.New("temporary connection error")))
}

func TestGroupErrorBeforeAssignment(t *testing.T) {
	for _, err := range []error{kerr.GroupAuthorizationFailed, kerr.TopicAuthorizationFailed, kerr.InvalidGroupID, kerr.InconsistentGroupProtocol, kerr.GroupMaxSizeReached} {
		c, _ := fakeConsumer(t)
		c.owned = make(map[partitionKey]bool)
		clientErrors{c}.OnGroupManageError(err)
		require.ErrorIs(t, c.result(), err)
	}
	c, _ := fakeConsumer(t)
	c.owned = make(map[partitionKey]bool)
	clientErrors{c}.OnGroupManageError(io.EOF)
	require.NoError(t, c.result())
}

func TestGroupSessionTerminationCancelsInflight(t *testing.T) {
	c, _ := fakeConsumer(t)
	c.state = running
	clientErrors{c}.OnGroupManageError(nil)
	clientErrors{c}.OnGroupManageError(context.Canceled)
	require.NoError(t, c.result())
	clientErrors{c}.OnGroupManageError(kerr.UnknownMemberID)
	require.ErrorIs(t, c.result(), kerr.UnknownMemberID)
	require.ErrorIs(t, context.Cause(c.ctx), kerr.UnknownMemberID)
	c.finish()
}
