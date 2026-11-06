package kafka

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"serv_tpl/pkg/logger"
)

// wireProxy 只修改指定 ClientID 的网络流量，Broker 与 SDK 均为真实实例。
// 每个请求通过 correlation ID 对应响应，支持并发和重新建连。
type wireProxy struct {
	listener           net.Listener
	upstream           string
	target             string
	dropCommitRequest  atomic.Bool
	dropCommitResponse atomic.Bool
	dropHeartbeat      atomic.Bool
	commitRequests     atomic.Int32
	commitResponses    atomic.Int32
	heartbeats         atomic.Int32
	mu                 sync.Mutex
	connections        map[net.Conn]struct{}
	closed             bool
	wg                 sync.WaitGroup
}

func newWireProxy(t *testing.T, upstream string) *wireProxy {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &wireProxy{listener: l, upstream: upstream, target: "d22-fault-target", connections: make(map[net.Conn]struct{})}
	p.launch(func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				_ = conn.Close()
				return
			}
			p.connections[conn] = struct{}{}
			p.mu.Unlock()
			p.launch(func() { p.serve(conn) })
		}
	})
	t.Cleanup(p.close)
	return p
}

func (p *wireProxy) launch(fn func()) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() {
			if value := recover(); value != nil {
				logger.L().Error("Kafka 测试代理 panic", zap.Any("panic", value), zap.ByteString("stack", debug.Stack()))
			}
		}()
		fn()
	}()
}

func (p *wireProxy) close() {
	p.mu.Lock()
	p.closed = true
	_ = p.listener.Close()
	for conn := range p.connections {
		_ = conn.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *wireProxy) disconnectAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for conn := range p.connections {
		_ = conn.Close()
	}
}

type wireRequest struct {
	api    int16
	target bool
}

func (p *wireProxy) serve(downstream net.Conn) {
	defer func() { _ = downstream.Close(); p.mu.Lock(); delete(p.connections, downstream); p.mu.Unlock() }()
	upstream, err := net.DialTimeout("tcp", p.upstream, 2*time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.connections[upstream] = struct{}{}
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.connections, upstream); p.mu.Unlock() }()
	var requestMu sync.Mutex
	var blackhole atomic.Bool
	requests := make(map[int32]wireRequest)
	responsesDone := make(chan struct{})
	p.launch(func() {
		defer close(responsesDone)
		defer downstream.Close()
		for {
			frame, err := readWireFrame(upstream)
			if err != nil {
				return
			}
			if len(frame) < 8 {
				return
			}
			correlation := int32(binary.BigEndian.Uint32(frame[4:8]))
			requestMu.Lock()
			request := requests[correlation]
			delete(requests, correlation)
			requestMu.Unlock()
			if blackhole.Load() {
				continue
			}
			if request.target && request.api == 8 && p.dropCommitResponse.Load() {
				p.commitResponses.Add(1)
				blackhole.Store(true)
				continue
			}
			if err := writeWireFrame(downstream, frame); err != nil {
				return
			}
		}
	})
	defer func() { _ = upstream.Close(); <-responsesDone }()
	for {
		frame, err := readWireFrame(downstream)
		if err != nil {
			return
		}
		if len(frame) < 14 {
			return
		}
		api := int16(binary.BigEndian.Uint16(frame[4:6]))
		correlation := int32(binary.BigEndian.Uint32(frame[8:12]))
		clientLen := int(int16(binary.BigEndian.Uint16(frame[12:14])))
		isTarget := clientLen >= 0 && len(frame) >= 14+clientLen && string(frame[14:14+clientLen]) == p.target
		if blackhole.Load() {
			continue
		}
		if isTarget && api == 8 {
			p.commitRequests.Add(1)
			if p.dropCommitRequest.Load() {
				blackhole.Store(true)
				continue
			}
		}
		if isTarget && api == 12 && p.dropHeartbeat.Load() {
			p.heartbeats.Add(1)
			blackhole.Store(true)
			continue
		}
		requestMu.Lock()
		requests[correlation] = wireRequest{api, isTarget}
		requestMu.Unlock()
		if err := writeWireFrame(upstream, frame); err != nil {
			return
		}
	}
}

func readWireFrame(conn net.Conn) ([]byte, error) {
	if err := conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
		return nil, err
	}
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(header[:]))
	if size < 4 || size > 32<<20 {
		return nil, fmt.Errorf("invalid Kafka frame length %d", size)
	}
	frame := make([]byte, size+4)
	copy(frame, header[:])
	_, err := io.ReadFull(conn, frame[4:])
	return frame, err
}

func writeWireFrame(conn net.Conn, frame []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	for len(frame) > 0 {
		n, err := conn.Write(frame)
		if err != nil {
			return err
		}
		frame = frame[n:]
	}
	return nil
}
