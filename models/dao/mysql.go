package dao

import (
	"fmt"
	"gtank/internal/conf"
	"gtank/pkg/glog"
	"log"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var MysqlCli *gorm.DB
var MdbPrefix string

func MustInitMysql() {
	c := conf.AppConf
	// 读配置
	dsn := c.GetString("mysql.dsn")
	MdbPrefix = c.GetString("mysql.prefix")
	isDebug := c.GetBool("mysql.debug")
	maxIdleConns := c.GetInt("mysql.maxIdleConns")
	maxOpenConns := c.GetInt("mysql.maxOpenConns")
	connMaxLifetime := c.GetInt("mysql.connMaxLifetime")

	db, err := ConnMysql(dsn, isDebug)
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

func ConnMysql(dsn string, isDebug bool) (db *gorm.DB, err error) {
	w := &MmyLog{
		logger: glog.D().Z().With(zap.String("type", "sql_log")),
	}
	logger := logger.New(w, logger.Config{
		SlowThreshold: time.Millisecond * 200,
		LogLevel:      logger.Silent,
		Colorful:      false,
	})
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger,
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
	if err != nil {
		return
	}
	return
}

func GetTmpMysql(dsn string) (db *gorm.DB, err error) {
	return ConnMysql(dsn, false)
}

func CloseTmpMysql(db *gorm.DB) {
	mdb, err := db.DB()
	if err != nil {
		return
	}
	mdb.Close()
}

// 接管mysql 日志
type MmyLog struct {
	logger *zap.Logger
}

func (l *MmyLog) Printf(tpl string, args ...interface{}) {
	tpl = strings.ReplaceAll(tpl, "\n", " ")
	msg := "[sql] " + fmt.Sprintf(tpl, args...)
	if _, ok := args[1].(error); ok {
		l.logger.Error(msg)
	} else {
		l.logger.Info(msg)
	}
}
