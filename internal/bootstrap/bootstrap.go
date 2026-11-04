// Package bootstrap 负责进程级资源装配及生命周期编排。
package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"serv_tpl/internal/global"
	"serv_tpl/models/dao"
	"serv_tpl/pkg/logger"
)

// MustInitLog 唯一装载全局日志；返回可报告错误的幂等清理函数。
func MustInitLog() func(context.Context) error {
	c := global.AppConf
	var h *logger.Holder
	var err error
	if c.GetString("log.type") == "file" {
		name := c.GetString("log.path")
		if name == "" {
			panic("log.path 未配置")
		}
		if err = os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			panic("日志目录创建失败")
		}
		h, err = logger.NewFileLogger(name, c.GetString("log.level"))
	} else {
		h, err = logger.NewStdLogger(c.GetString("log.level"))
	}
	if err != nil {
		panic("日志初始化失败")
	}
	logger.SetGlobal(h)
	var once sync.Once
	var closeErr error
	return func(context.Context) error { once.Do(func() { closeErr = h.Close() }); return closeErr }
}

// MustInitDBs 在总初始化期限内建立存储连接；失败直接 panic，不回收已成功的其他组件。
// 底层 DAO 仍负责释放自身构造失败的资源，业务补偿由业务层处理。
func MustInitDBs(timeout time.Duration) func(context.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := global.AppConf
	db, err := dao.OpenMysqlContext(ctx, dao.MysqlOptions{
		DSN: c.GetString("mysql.dsn"), MaxIdleConns: c.GetInt("mysql.maxIdleConns"),
		MaxOpenConns: c.GetInt("mysql.maxOpenConns"), ConnMaxLifetime: c.GetInt("mysql.connMaxLifetime"), Debug: c.GetBool("mysql.debug"),
	})
	if err != nil {
		panic(err)
	}
	r, err := dao.OpenRedisContext(ctx, &redis.Options{
		Addr: c.GetString("redis.addr"), Password: c.GetString("redis.pwd"), DB: c.GetInt("redis.db"),
		PoolSize: c.GetInt("redis.pool_size"), MaxRetries: c.GetInt("redis.max_reties"),
		DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, PoolTimeout: 4 * time.Second,
		ContextTimeoutEnabled: true,
	})
	if err != nil {
		panic(err)
	}
	if err = ctx.Err(); err != nil {
		panic(err)
	}
	global.MysqlCli, global.RedisCli = db, r
	var once sync.Once
	var closeErr error
	return func(context.Context) error {
		once.Do(func() { closeErr = errors.Join(dao.CloseRedis(r), dao.CloseMysql(db)) })
		return closeErr
	}
}

func logPanic(component string) {
	logger.L().Error("组件 panic", zap.String("component", component), zap.Stack("stack"))
}
