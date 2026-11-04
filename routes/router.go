package routes

import (
	controller "serv_tpl/controller"
	v1 "serv_tpl/controller/v1"
	"serv_tpl/internal/global"

	"github.com/gin-gonic/gin"
)

// RegisterHTTP 集中注册 HTTP 路由，供启动装配在监听前调用。
func RegisterHTTP(r *gin.Engine, ready func() bool) {
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

func registAdmin(r *gin.Engine) {

	userRout := r.Group("/admin/user").
		Use(
		// 绑定在特定路由组下的中间件
		)

	u := v1.User{}
	userRout.POST("/create", u.Create)
	userRout.GET("/list", u.List)
}
