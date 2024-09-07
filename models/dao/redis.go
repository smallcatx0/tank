package dao

import (
	"context"
	"gtank/internal/conf"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	Rdb         *redis.Client
	CachePrefix string
)

func InitRedis() error {
	c := conf.AppConf
	CachePrefix = c.GetString("redis.prefix")
	return ConnRedis(&redis.Options{
		Addr:       c.GetString("redis.addr"),
		DB:         c.GetInt("redis.db"),
		Password:   c.GetString("redis.pwd"),
		PoolSize:   c.GetInt("redis.pool_size"),
		MaxRetries: c.GetInt("redis.max_reties"),
	})
}

func ConnRedis(opt *redis.Options) error {
	Rdb = redis.NewClient(opt)
	ctx := context.Background()
	_, err := Rdb.Ping(ctx).Result()
	if err != nil {
		zap.L().Error("[dao] redis fail, err=" + err.Error())
		return err
	}
	return nil
}
