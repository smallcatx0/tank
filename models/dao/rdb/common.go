package rdb

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"gtank/models/dao"
)

// K key拼接
func K(keys ...string) string {
	curr := 0
	// 除去所有空白部分
	for _, v := range keys {
		if v != "" {
			keys[curr] = v
			curr += 1
		}
	}
	return strings.Join(keys[0:curr], ":")
}

// BlurTTL 随机过期时间
func BlurTTL(sec int) int {
	if sec < 300 {
		return sec
	}
	rand.Seed(time.Now().UnixNano())
	return sec - 30 + rand.Intn(60)
}

// RateLimit 流量控制
func RateLimit(key string, sec, max int) bool {
	rdb := dao.RedisCli
	res := rdb.Get(context.Background(), key)
	if res.Err() != nil {
		// 如果没有此key 创建 过期时间为time =》 true
		rdb.Set(context.Background(), key, 1, time.Second*time.Duration(sec))
		return true
	}
	curr, _ := res.Int()
	if max > curr {
		rdb.Incr(context.Background(), key)
		return true
	}
	return false
}

func Remember(key string, f func() string, exp ...int) (string, error) {
	key = K(dao.CachePrefix, key)
	cli := dao.RedisCli
	ret, err := cli.Get(context.Background(), key).Result()
	if err == nil {
		return ret, nil
	}
	expSec := 300
	if len(exp) >= 1 {
		expSec = exp[0]
	}
	expSec = BlurTTL(expSec)
	ret = f()
	err = cli.Set(context.Background(), key, ret, time.Second*time.Duration(expSec)).Err()
	return ret, err
}
