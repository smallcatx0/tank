package dao

import (
	"gtank/internal/conf"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var MDB *gorm.DB

func InitMysql() {
	c := conf.AppConf
	// 读配置
	dsn := c.GetString("mysql.dsn")
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
	MDB = db
}

func ConnMysql(dsn string, isDebug bool) (db *gorm.DB, err error) {
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
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
