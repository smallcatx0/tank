package kafka

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

type faultFixture struct {
	name, address, authAddress, password string
	proxy                                *wireProxy
	tls                                  *tls.Config
	admin                                *kgo.Client
}

func dockerTestCommand(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	result := string(output)
	for _, arg := range args {
		if strings.Contains(arg, "password=") {
			result = strings.ReplaceAll(result, arg, "[测试凭据已省略]")
		}
	}
	require.NoError(t, err, "docker %s: %s", args[0], result)
	return strings.TrimSpace(result)
}

func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, listener.Close())
	return port
}

func fixtureCertificate(t *testing.T) ([]byte, []byte, *tls.Config) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "D22 test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	server := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rand.Reader, server, ca, &serverKey.PublicKey, caKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	require.NoError(t, err)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	chain := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), caPEM...)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caPEM))
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), chain, &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

func startFaultFixture(t *testing.T) *faultFixture {
	t.Helper()
	// 显式开关授权测试自行创建、重启和删除专用容器，不接触常驻 Kafka。
	if os.Getenv("KAFKA_DOCKER_TESTS") != "1" {
		t.Skip("设置 KAFKA_DOCKER_TESTS=1 后运行 Docker 认证与故障注入")
	}
	port, authPort := freePort(t), freePort(t)
	f := &faultFixture{name: fmt.Sprintf("gtank-d22-fault-%d", time.Now().UnixNano()), address: "127.0.0.1:" + port, authAddress: "127.0.0.1:" + authPort}
	f.proxy = newWireProxy(t, f.address)
	key, chain, tlsConfig := fixtureCertificate(t)
	f.tls = tlsConfig
	var secret [16]byte
	_, err := rand.Read(secret[:])
	require.NoError(t, err)
	f.password = hex.EncodeToString(secret[:])
	properties := fmt.Sprintf(`process.roles=broker,controller
node.id=1
controller.quorum.voters=1@localhost:9095
controller.listener.names=CONTROLLER
listeners=INTERNAL://:9094,PUBLIC://:9092,AUTH://:9093,CONTROLLER://:9095
advertised.listeners=INTERNAL://localhost:9094,PUBLIC://%s,AUTH://%s
listener.security.protocol.map=INTERNAL:PLAINTEXT,PUBLIC:PLAINTEXT,AUTH:SASL_SSL,CONTROLLER:PLAINTEXT
inter.broker.listener.name=INTERNAL
log.dirs=/tmp/d22-kafka-data
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
group.initial.rebalance.delay.ms=0
group.min.session.timeout.ms=1000
auto.create.topics.enable=false
authorizer.class.name=org.apache.kafka.metadata.authorizer.StandardAuthorizer
super.users=User:ANONYMOUS;User:d22user
allow.everyone.if.no.acl.found=false
listener.name.auth.sasl.enabled.mechanisms=PLAIN,SCRAM-SHA-256,SCRAM-SHA-512
listener.name.auth.plain.sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required user_d22user="%s" user_denied="%s";
listener.name.auth.scram-sha-256.sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required;
listener.name.auth.scram-sha-512.sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required;
ssl.keystore.type=PEM
ssl.keystore.key=%s
ssl.keystore.certificate.chain=%s
ssl.client.auth=none
`, f.proxy.listener.Addr(), f.authAddress, f.password, f.password, strings.ReplaceAll(string(key), "\n", `\n`), strings.ReplaceAll(string(chain), "\n", `\n`))
	config := filepath.Join(t.TempDir(), "server.properties")
	require.NoError(t, os.WriteFile(config, []byte(properties), 0600))
	dockerTestCommand(t, "create", "--name", f.name, "--user", "0", "-p", "127.0.0.1:"+port+":9092", "-p", "127.0.0.1:"+authPort+":9093", "--entrypoint", "/bin/bash", "apache/kafka:4.1.1", "-c", "set -e; /opt/kafka/bin/kafka-storage.sh format --ignore-formatted -t MkU3OEVBNTcwNTJENDM2Qk -c /tmp/d22.properties; exec /opt/kafka/bin/kafka-server-start.sh /tmp/d22.properties")
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(dockerTestCommand(t, "logs", "--tail", "40", f.name))
		}
		dockerTestCommand(t, "rm", "-f", "-v", f.name)
	})
	dockerTestCommand(t, "cp", config, f.name+":/tmp/d22.properties")
	dockerTestCommand(t, "start", f.name)
	f.admin, err = kgo.NewClient(kgo.SeedBrokers(f.address), kgo.ClientID("d22-admin"), kgo.RecordPartitioner(kgo.ManualPartitioner()), kgo.DialTimeout(time.Second), kgo.RequestTimeoutOverhead(time.Second), kgo.RecordDeliveryTimeout(10*time.Second))
	require.NoError(t, err)
	t.Cleanup(f.admin.Close)
	f.ready(t)
	for _, mechanism := range []string{"SCRAM-SHA-256", "SCRAM-SHA-512"} {
		dockerTestCommand(t, "exec", f.name, "/opt/kafka/bin/kafka-configs.sh", "--bootstrap-server", "localhost:9094", "--alter", "--entity-type", "users", "--entity-name", "d22user", "--add-config", fmt.Sprintf("%s=[iterations=4096,password=%s]", mechanism, f.password))
	}
	t.Log("Kafka 4.1.1 专项环境已就绪（TLS、PLAIN、SCRAM-SHA-256/512、独立协议代理）")
	return f
}

func (f *faultFixture) ready(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return f.admin.Ping(ctx) == nil
	}, 40*time.Second, 100*time.Millisecond)
}

func (f *faultFixture) options(t *testing.T) Options {
	t.Helper()
	o := DefaultOptions()
	o.Brokers = []string{f.proxy.listener.Addr().String()}
	o.Topics = []string{fmt.Sprintf("d22-%d", time.Now().UnixNano())}
	o.GroupID = o.Topics[0]
	o.ClientID = f.proxy.target
	o.StartOffset = "earliest"
	o.AttemptTimeout = time.Second
	o.MessageTimeout = 2 * time.Second
	o.CommitTimeout = 2 * time.Second
	o.Advanced.PollTimeout = 100 * time.Millisecond
	o.Advanced.RequestTimeoutOverhead = time.Second
	o.Advanced.RequestRetryTimeout = 3 * time.Second
	o.Advanced.Group = GroupOptions{3 * time.Second, 500 * time.Millisecond, 15 * time.Second}
	request := kmsg.NewPtrCreateTopicsRequest()
	request.Topics = []kmsg.CreateTopicsRequestTopic{{Topic: o.Topics[0], NumPartitions: 1, ReplicationFactor: 1}}
	request.TimeoutMillis = 5000
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := request.RequestWith(ctx, f.admin)
	require.NoError(t, err)
	require.Zero(t, response.Topics[0].ErrorCode)
	return o
}

func committedOffset(t *testing.T, cl *kgo.Client, o Options) int64 {
	t.Helper()
	request := kmsg.NewPtrOffsetFetchRequest()
	request.Group = o.GroupID
	request.Topics = []kmsg.OffsetFetchRequestTopic{{Topic: o.Topics[0], Partitions: []int32{0}}}
	// 同时提供新旧协议字段，由 SDK 按 Broker 协商版本编码。
	request.Groups = []kmsg.OffsetFetchRequestGroup{{Group: o.GroupID, Topics: []kmsg.OffsetFetchRequestGroupTopic{{Topic: o.Topics[0], Partitions: []int32{0}}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := request.RequestWith(ctx, cl)
	require.NoError(t, err)
	if len(response.Groups) > 0 {
		require.Zero(t, response.Groups[0].ErrorCode)
		require.Len(t, response.Groups[0].Topics, 1)
		p := response.Groups[0].Topics[0].Partitions[0]
		require.Zero(t, p.ErrorCode)
		return p.Offset
	}
	require.Zero(t, response.ErrorCode)
	require.Len(t, response.Topics, 1)
	p := response.Topics[0].Partitions[0]
	require.Zero(t, p.ErrorCode)
	return p.Offset
}

func awaitRun(t *testing.T, done <-chan error, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		t.Fatal("Run 未在预期时间内返回")
		return nil
	}
}
