package dao

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"time"
)

// OpenRedis 兼容入口，初始化最多等待 15 秒。
func OpenRedis(opt *redis.Options) (*redis.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return OpenRedisContext(ctx, opt)
}

// OpenRedisContext 建立独立客户端；Ping 失败立即回收，不输出连接凭据。
func OpenRedisContext(ctx context.Context, opt *redis.Options) (*redis.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opt == nil {
		return nil, errors.New("redis 配置为空")
	}
	o := *opt
	if o.DialTimeout <= 0 {
		o.DialTimeout = 3 * time.Second
	}
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = 3 * time.Second
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = 3 * time.Second
	}
	if o.PoolTimeout <= 0 {
		o.PoolTimeout = 4 * time.Second
	}
	if o.PoolSize <= 0 {
		o.PoolSize = 30
	}
	o.ContextTimeoutEnabled = true
	cli := redis.NewClient(&o)
	if err := cli.Ping(ctx).Err(); err != nil {
		_ = cli.Close()
		return nil, errors.New("redis 初始化连接失败")
	}
	if err := ctx.Err(); err != nil {
		_ = cli.Close()
		return nil, err
	}
	return cli, nil
}

// CloseRedis 关闭客户端，nil 安全。
func CloseRedis(cli *redis.Client) error {
	if cli == nil {
		return nil
	}
	return cli.Close()
}
