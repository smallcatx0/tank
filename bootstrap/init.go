package bootstrap

import (
	"gitee.com/smallcatx0/gtank/models/dao"
	"gitee.com/smallcatx0/gtank/models/data"
	"gitee.com/smallcatx0/gtank/pkg/conf"
	"gitee.com/smallcatx0/gtank/pkg/glog"
)

// InitConf 配置文件初始化
func InitConf(filePath *string) {
	err := conf.InitAppConf(filePath)
	if err != nil {
		panic(err)
	}
}

// initLog 初始化日志
func InitLog() {
	c := conf.AppConf
	if c.GetString("log.type") == "file" {
		glog.InitLog2file(
			c.GetString("log.path"),
			c.GetString("log.level"),
		)
	} else {
		glog.InitLog2std(c.GetString("log.level"))
	}
}

// InitDB 初始化db
func InitDB() {
	// dao.MysqlInit()
	err := dao.InitRedis()
	if err != nil {
		panic(err)
	}
	data.InitSub(1)
}
