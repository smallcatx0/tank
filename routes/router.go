package routes

import (
	"gitee.com/smallcatx0/gtank/controller"
	"gitee.com/smallcatx0/gtank/pkg/conf"

	"github.com/gin-gonic/gin"
)

// Register http路由总入口
func Register(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		v := conf.AppConf.GetString("base.describe")
		c.String(200, v)
	}) // version
	health := controller.Health{}
	r.GET("/healthz", health.Healthz)
	r.GET("/ready", health.Ready)
	r.GET("/reload", health.ReloadConf)
	r.GET("/test", health.Test)
	registeRoute(r)
}
