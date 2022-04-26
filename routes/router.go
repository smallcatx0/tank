package routes

import (
	"gtank/controller"
	"gtank/internal/conf"

	"github.com/gin-gonic/gin"
)

// Register http路由总入口
func Register(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		v := conf.AppConf.GetString("base.describe")
		c.String(200, v)
	}) // version
	health := controller.Health{}
	{
		r.GET("/healthz", health.Healthz)
		r.GET("/ready", health.Ready)
		r.GET("/reload", health.ReloadConf)
		r.GET("/test", health.Test)
	}

	// 注册对外接口
	registeApi(r)
	// 注册admin接口
	registAdmin(r)
}
