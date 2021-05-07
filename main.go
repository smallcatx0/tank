package main

import (
	"gitee.com/smallcatx0/gtank/bootstrap"
	"gitee.com/smallcatx0/gtank/middleware/httpmd"
	"gitee.com/smallcatx0/gtank/pkg/conf"
	"gitee.com/smallcatx0/gtank/routes"
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
