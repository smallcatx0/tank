package main

import (
	"gtank/bootstrap"
	"gtank/internal/task"
	"gtank/models/dao"
)

func main() {
	bootstrap.InitFlag()
	if !bootstrap.Flag() {
		return
	}
	// 读取配置文件
	bootstrap.InitConf(&bootstrap.Param.C)
	// 初始化 数据库
	dao.MustInitRedis()
	dao.MustInitMysql()
	// 初始化日志
	bootstrap.InitLog()
	// 心跳日志记录
	bootstrap.Heartbeat()

	dbClose := task.StartSthTask()
	// rmqClose := task.StartRmqTask()

	// 等待推出信号
	bootstrap.WaitingExit(
		dbClose,
		// rmqClose,
	)
}
