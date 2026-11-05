package main

import (
	"serv_tpl/bootstrap"
	"serv_tpl/internal/conf"
	"serv_tpl/internal/httpmd"
	"serv_tpl/routes"

	"github.com/gin-gonic/gin"
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
	bootstrap.InitLog()
	bootstrap.InitDB()
	bootstrap.Heartbeat()

	app := bootstrap.NewApp(conf.IsDebug())
	app.GinEngibe.Use(httpmd.SetHeader)
	app.GinEngibe.Use(httpmd.ReqLogFull)
	// 注册路由（min_tpl 签名带 ready 回调，兼容入口恒传 true）
	app.RegisterRoutes(func(r *gin.Engine) { routes.RegisterHTTP(r, func() bool { return true }) })
	// 启动HTTP 服务
	app.Run(conf.HttpPort())
	// 等待退出
	app.WaitExit()
}
