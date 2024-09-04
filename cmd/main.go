package main

import (
	"gtank/bootstrap"
	"gtank/internal/conf"
	"gtank/middleware/httpmd"
	"gtank/routes"
)

func init() {
	bootstrap.InitFlag()
}

func main() {
	if !bootstrap.Flag() {
		return
	}
	// 读取配置文件
	bootstrap.InitConf(&bootstrap.Param.C)
	app := bootstrap.NewApp(conf.IsDebug())
	// 初始化操作
	app.Use(
		bootstrap.InitLog,
		bootstrap.InitDB,
		bootstrap.Heartbeat,
		bootstrap.InitComsumer,
	)
	app.GinEngibe.Use(httpmd.SetHeader)
	app.GinEngibe.Use(httpmd.ReqLog)
	// 注册路由
	app.RegisterRoutes(routes.Register)
	// 启动HTTP 服务
	app.Run(conf.HttpPort())
	// 等待退出
	app.WaitExit()
}
