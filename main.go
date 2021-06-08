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
	// 读取配置文件
	bootstrap.InitConf(&bootstrap.Param.C)
	app := bootstrap.NewApp(conf.IsDebug())
	// 初始化操作
	app.Use(bootstrap.InitLog, bootstrap.InitDB)
	app.GinEngibe.Use(httpmd.SetHeader)
	app.GinEngibe.Use(httpmd.ReqLog)
	// 注册路由
	app.RegisterRoutes(routes.Register)
	// 启动HTTP 服务
	app.Run(conf.HttpPort())
}
