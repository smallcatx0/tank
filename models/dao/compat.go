// dao 兼容层：保留 master 分支“全局实例 + 自读配置”的老装配模式（*Cli 全局变量 + MustInit*），
// 供业务代码（models/dao/rds、internal/task、controller 等）沿用；
// 新代码请优先使用 mysql.go / redis.go 中依赖注入风格（Options 传入、返回实例）的入口。
package dao

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"serv_tpl/internal/conf"
	"serv_tpl/pkg/glog"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var MysqlCli *gorm.DB
var MdbPrefix string

// MustInitMysql 从配置初始化全局 MySQL 实例（老模式，失败即 Panic）
func MustInitMysql() {
	c := conf.AppConf
	// 读配置
	dsn := c.GetString("mysql.dsn")
	MdbPrefix = c.GetString("mysql.prefix")
	isDebug := c.GetBool("mysql.debug")
	maxIdleConns := c.GetInt("mysql.maxIdleConns")
	maxOpenConns := c.GetInt("mysql.maxOpenConns")
	connMaxLifetime := c.GetInt("mysql.connMaxLifetime")

	db, err := connMysqlDebug(dsn, isDebug)
	if err != nil {
		log.Panic("[store_mysql] conn mysql fail err=", err)
	}
	mdb, _ := db.DB()
	mdb.SetMaxIdleConns(maxIdleConns)
	mdb.SetMaxOpenConns(maxOpenConns)
	mdb.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)
	// 赋给全局变量
	MysqlCli = db
}

// connMysqlDebug 带调试开关的连接入口，与 dao.ConnMysql(dsn) 区分
func connMysqlDebug(dsn string, isDebug bool) (db *gorm.DB, err error) {
	w := &ZapWriter{glog.L().With(zap.String("type", "sql_log"))}
	lg := gormlogger.New(w, gormlogger.Config{
		SlowThreshold: time.Millisecond * 200,
		LogLevel:      gormlogger.Silent,
		Colorful:      false,
	})
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: lg,
	})
	if err != nil {
		return
	}
	if isDebug {
		db = db.Debug()
	}
	mdb, err := db.DB()
	if err != nil {
		return
	}
	err = mdb.Ping()
	return
}

// GetTmpMysql 临时连接（单测等场景），用完需 CloseTmpMysql 回收
func GetTmpMysql(dsn string) (db *gorm.DB, err error) {
	return connMysqlDebug(dsn, false)
}

// CloseTmpMysql 关闭临时连接池
func CloseTmpMysql(db *gorm.DB) {
	mdb, err := db.DB()
	if err != nil {
		return
	}
	mdb.Close()
}

var (
	RedisCli    *redis.Client
	CachePrefix string
)

// MustInitRedis 从配置初始化全局 Redis 实例（老模式，失败即 Panic）
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
}

// ConnRedis 建立 Redis 连接并 Ping 验证
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

// ZapWriter 接管 gorm 的 sql 日志，写入 glog 全局实例
type ZapWriter struct {
	*zap.Logger
}

func (w *ZapWriter) Printf(tpl string, args ...interface{}) {
	tpl = strings.ReplaceAll(tpl, "\n", " ")
	msg := "[sql] " + fmt.Sprintf(tpl, args...)
	if len(args) > 1 {
		if _, ok := args[1].(error); ok {
			w.Error(msg)
			return
		}
	}
	w.Info(msg)
}
