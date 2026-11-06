package main

import (
	"context"
	"testing"
	"time"
)

func TestCallVersionInvalidOptions(t *testing.T) {
	for _, tc := range []struct {
		addr    string
		timeout time.Duration
	}{{"", time.Second}, {"127.0.0.1:9090", 0}, {"127.0.0.1:9090", -time.Second}} {
		if err := callVersion(context.Background(), tc.addr, tc.timeout); err == nil {
			t.Errorf("非法参数应拒绝：addr=%q timeout=%s", tc.addr, tc.timeout)
		}
	}
}
