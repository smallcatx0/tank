package main

import (
	"tank/bootstrap"
	"tank/middleware/httpmd"
	"tank/pkg/conf"
	"tank/routes"
)

func init() {
	bootstrap.InitFlag()
}

func main() {
	if !bootstrap.Flag() {
		return
	}
	bootstrap.InitConf(&bootstrap.Param.C)
	app := bootstrap.NewApp(conf.IsDebug())
	// 初始化操作
	app.Use(bootstrap.InitLog, bootstrap.InitDB)
	app.GinEngibe.Use(httpmd.SetHeader)
	// 注册路由
	app.RegisterRoutes(routes.Init)
	// 启动HTTP 服务
	app.Run(conf.HttpPort())
}
