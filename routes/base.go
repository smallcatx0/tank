package routes

import (
	C "tank/controller"
	"tank/pkg/conf"

	"github.com/gin-gonic/gin"
)

// Init http路由总入口
func Init(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		v := conf.AppConf.GetString("base.describe")
		c.String(200, v)
	}) // version
	r.GET("/healthz", C.Healthz)
	r.GET("/ready", C.Ready)
	r.GET("/reload", C.ReloadConf)
	r.GET("/test", C.Test)
	registeRoute(r)
}
