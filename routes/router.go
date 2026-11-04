package routes

import (
	controller "serv_tpl/controller"
	"serv_tpl/internal/global"

	"github.com/gin-gonic/gin"
)

// Register http路由总入口
func Register(r *gin.Engine, ready func() bool) {
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"name":    global.AppConf.GetString("name"),
			"desc":    global.AppConf.GetString("desc"),
			"version": global.AppConf.GetString("ver"),
		})
	}) // version
	health := controller.Health{IsReady: ready}
	{
		r.GET("/healthz", health.Healthz)
		r.GET("/ready", health.Ready)

	}
	// 注册admin接口
	registAdmin(r)
	// 注册Open API接口

}
