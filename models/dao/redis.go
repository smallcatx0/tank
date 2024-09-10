package dao

import (
	"context"
	"gtank/internal/conf"
	"log"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	RedisCli    *redis.Client
	CachePrefix string
)

/*
初始化Redis客户端连接。
这个函数从配置中读取Redis的相关参数，然后尝试建立连接。
如果连接失败，函数会记录panic日志并终止程序。
*/
func MustInitRedis() {
	c := conf.AppConf
	CachePrefix = c.GetString("redis.prefix")
	var err error
	RedisCli, err = ConnRedis(&redis.Options{
		Addr:       c.GetString("redis.addr"),
		DB:         c.GetInt("redis.db"),
		Password:   c.GetString("redis.pwd"),
		PoolSize:   c.GetInt("redis.pool_size"),
		MaxRetries: c.GetInt("redis.max_reties"),
	})
	if err != nil {
		log.Panic("[store_redis]redis初始化失败，err=", err)
	}
	return
}

func ConnRedis(opt *redis.Options) (*redis.Client, error) {
	cli := redis.NewClient(opt)
	ctx := context.Background()
	_, err := cli.Ping(ctx).Result()
	if err != nil {
		zap.L().Error("[dao] redis fail, err=" + err.Error())
		return nil, err
	}
	return cli, nil
}
