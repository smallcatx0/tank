package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// clientErrors 接收不一定通过 PollRecords 暴露的不可恢复连接和组错误。
// Hook 只发布停止信号，不在 SDK 的锁或回调中关闭客户端。
type clientErrors struct{ consumer *Consumer }

// OnBrokerConnect 将 TLS 校验或 SASL 初始化失败传递给 Run；普通断连仍由 SDK 重试。
func (h clientErrors) OnBrokerConnect(_ kgo.BrokerMetadata, _ time.Duration, _ net.Conn, err error) {
	defer h.recoverHook()
	if fatalAuthentication(err) {
		h.fail(err)
	}
}

// OnGroupManageError 保留组授权错误；已有分区时，组会话终止必须立即阻止旧消息提交。
func (h clientErrors) OnGroupManageError(err error) {
	defer h.recoverHook()
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	c := h.consumer
	c.mu.Lock()
	hadAssignment := len(c.owned) > 0 && (c.state == running || c.state == draining)
	c.mu.Unlock()
	if hadAssignment || fatalAuthentication(err) || errors.Is(err, kerr.GroupAuthorizationFailed) || errors.Is(err, kerr.TopicAuthorizationFailed) || errors.Is(err, kerr.InvalidGroupID) || errors.Is(err, kerr.InconsistentGroupProtocol) || errors.Is(err, kerr.GroupMaxSizeReached) {
		h.fail(err)
	}
}

func fatalAuthentication(err error) bool {
	if err == nil {
		return false
	}
	var verification *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	return errors.Is(err, kerr.SaslAuthenticationFailed) || errors.Is(err, kerr.UnsupportedSaslMechanism) || errors.Is(err, kerr.IllegalSaslState) || errors.As(err, &verification) || errors.As(err, &unknown) || errors.As(err, &hostname) || errors.As(err, &invalid)
}

func (h clientErrors) fail(err error) {
	h.consumer.stop(&ConsumeError{Stage: "group", Partition: -1, Offset: -1, Err: err})
}

func (h clientErrors) recoverHook() {
	if value := recover(); value != nil {
		h.consumer.stop(h.consumer.panicError("group", value))
	}
}
