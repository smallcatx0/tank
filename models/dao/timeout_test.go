package dao

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"net"
	"testing"
	"time"
)

func TestHandshakeDeadline(t *testing.T) {
	for _, kind := range []string{"mysql", "redis"} {
		t.Run(kind, func(t *testing.T) {
			ln, e := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, e)
			defer ln.Close()
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				defer func() {
					if v := recover(); v != nil {
						t.Errorf("listener panic: %v", v)
					}
				}()
				conn, e := ln.Accept()
				if e != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetReadDeadline(time.Now().Add(time.Second))
				b := make([]byte, 4096)
				for {
					if _, e = conn.Read(b); e != nil {
						return
					}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			if kind == "mysql" {
				_, e = OpenMysqlContext(ctx, MysqlOptions{DSN: "u:p@tcp(" + ln.Addr().String() + ")/test"})
			} else {
				_, e = OpenRedisContext(ctx, &redis.Options{Addr: ln.Addr().String(), MaxRetries: -1})
			}
			require.Error(t, e)
			require.Less(t, time.Since(started), time.Second)
			_ = ln.Close()
			<-finished
		})
	}
}
