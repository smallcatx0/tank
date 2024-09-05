package bootstrap

import (
	"fmt"
	"gtank/internal/conf"
	"gtank/internal/task"
	"gtank/models/dao"
	"gtank/pkg/glog"
	"time"
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
	dao.InitMysql()
	err := dao.InitRedis()
	if err != nil {
		panic(err)
	}
}

// 心跳&状态检测
func Heartbeat() {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("Heartbeat goroutine run panic " + fmt.Sprint(r))
		}
	}()
	if !conf.AppConf.GetBool("metrics") {
		return
	}
	dt := conf.AppConf.GetInt("metrics_dt")
	if dt == 0 {
		dt = 10
	}
	go func() {
		ticker := time.NewTicker(time.Second * time.Duration(dt))
		defer ticker.Stop()
		for range ticker.C {
			glog.SysStatInfo()
		}
	}()
}

// 启动消费者
func InitComsumer() {
	task.StartSthTask()
}
