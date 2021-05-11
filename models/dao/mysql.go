package dao

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var MDB *gorm.DB

func MysqlInit() {

	gorm.Open(mysql.Open(dsn), &gorm.Config{})
}
