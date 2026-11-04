package dao

import (
	"context"
	"database/sql"
	"errors"

	driver "github.com/go-sql-driver/mysql"

	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

// MysqlOptions 连接参数：由装配层从配置解析后传入，dao 自身不读配置、不持有全局实例
type MysqlOptions struct {
	DSN             string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime int // 秒
	Debug           bool
}

// OpenMysql 保留兼容入口，所有初始化都有总期限。
func OpenMysql(o MysqlOptions) (*gorm.DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return OpenMysqlContext(ctx, o)
}

// OpenMysqlContext 用显式网络期限建池，初始化失败时回收。
func OpenMysqlContext(ctx context.Context, o MysqlOptions) (*gorm.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := driver.ParseDSN(o.DSN)
	if err != nil {
		return nil, errors.New("mysql DSN 无效")
	}
	cfg.Timeout = 3 * time.Second
	cfg.ReadTimeout = 3 * time.Second
	cfg.WriteTimeout = 3 * time.Second
	connector, err := driver.NewConnector(cfg)
	if err != nil {
		return nil, errors.New("mysql 连接配置无效")
	}
	pool := sql.OpenDB(connector)
	success := false
	defer func() {
		if !success {
			_ = pool.Close()
		}
	}()
	idle, open, lifetime := o.MaxIdleConns, o.MaxOpenConns, o.ConnMaxLifetime
	if idle <= 0 {
		idle = 10
	}
	if open <= 0 {
		open = 100
	}
	if lifetime <= 0 {
		lifetime = 300
	}
	pool.SetMaxIdleConns(idle)
	pool.SetMaxOpenConns(open)
	pool.SetConnMaxLifetime(time.Duration(lifetime) * time.Second)
	if err = pool.PingContext(ctx); err != nil {
		return nil, errors.New("mysql 初始化连接失败")
	}

	sqlLogger := sqlLogger{level: gormlog.Warn}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: sqlLogger, DisableAutomaticPing: true})
	if err != nil {
		return nil, errors.New("mysql ORM 初始化失败")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if o.Debug {
		db = db.Debug()
	}
	success = true
	return db, nil
}

// ConnMysql 兼容现有调用方，复用有界初始化。
func ConnMysql(dsn string) (*gorm.DB, error) { return OpenMysql(MysqlOptions{DSN: dsn}) }

// CloseMysql 关闭连接池，nil 安全
func CloseMysql(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	mdb, err := db.DB()
	if err != nil {
		return err
	}
	return mdb.Close()
}

// DsnMask 数据库链接脱敏, 对空/非法 DSN 做安全兜底, 避免解析越界 panic
func DsnMask(dsn string) string {
	if dsn == "" {
		return ""
	}
	info := strings.SplitN(dsn, "@", 2)
	if len(info) < 2 {
		// 不含 @ 分隔符, 无法定位账号密码, 整体脱敏
		return "******"
	}
	u := strings.SplitN(info[0], ":", 2)
	return u[0] + ":******@" + info[1]
}
