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
	"gorm.io/gorm"
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

// InitDBs 在局部完成全部初始化后发布连接；失败逆序回滚。
func InitDBs(ctx context.Context) (func(context.Context) error, error) {
	c := global.AppConf
	return initDBs(ctx,
		func(ctx context.Context) (*gorm.DB, error) {
			return dao.OpenMysqlContext(ctx, dao.MysqlOptions{
				DSN: c.GetString("mysql.dsn"), MaxIdleConns: c.GetInt("mysql.maxIdleConns"),
				MaxOpenConns: c.GetInt("mysql.maxOpenConns"), ConnMaxLifetime: c.GetInt("mysql.connMaxLifetime"), Debug: c.GetBool("mysql.debug"),
			})
		},
		func(ctx context.Context) (*redis.Client, error) {
			return dao.OpenRedisContext(ctx, &redis.Options{
				Addr: c.GetString("redis.addr"), Password: c.GetString("redis.pwd"), DB: c.GetInt("redis.db"),
				PoolSize: c.GetInt("redis.pool_size"), MaxRetries: c.GetInt("redis.max_reties"),
				DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, PoolTimeout: 4 * time.Second,
				ContextTimeoutEnabled: true,
			})
		}, dao.CloseMysql, dao.CloseRedis)
}

func initDBs(ctx context.Context, openDB func(context.Context) (*gorm.DB, error), openRedis func(context.Context) (*redis.Client, error), closeDB func(*gorm.DB) error, closeRedis func(*redis.Client) error) (stop func(context.Context) error, err error) {
	db, err := openDB(ctx)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			err = errors.Join(err, closeDB(db))
		}
	}()
	r, err := openRedis(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			err = errors.Join(err, closeRedis(r))
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	// 步骤1：全部成功后一次发布；清理捕获本次实例。
	global.MysqlCli, global.RedisCli = db, r
	success = true
	var once sync.Once
	var closeErr error
	return func(context.Context) error {
		once.Do(func() { closeErr = errors.Join(closeRedis(r), closeDB(db)) })
		return closeErr
	}, nil
}

func logPanic(component string) {
	logger.L().Error("组件 panic", zap.String("component", component), zap.Stack("stack"))
}
