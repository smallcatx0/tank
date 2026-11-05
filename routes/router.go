package routes

import (
	controller "serv_tpl/controller"
	v1 "serv_tpl/controller/v1"
	"serv_tpl/internal/global"
	"serv_tpl/internal/httpmd"

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
	// 注册admin接口（master 版用户后台，见 admin.go）
	registAdmin(r)
	// 注册Open API接口（master 版 v1 用户模块，见 api.go）
	registeApi(r)
}

// registAdminShop min_tpl 内联 shop 示例路由，与 admin.go 的 registAdmin 重名，改名共存
func registAdmin(r *gin.Engine) {
	s := v1.Shop{}

	shopRout := r.Group("/admin/shop").
		Use(
		// 绑定在特定路由组下的中间件
		)

	shopRout.POST("/create", s.Create)
	shopRout.GET("/list", s.List)

	userRout := r.Group("/user").Use(httpmd.JwtAuth())
	u := v1.UserAdmin{}
	userRout.GET("/list", u.List)
}

func registeApi(router *gin.Engine) {
	root := router.Group("/v1")

	userRout := root.Group("/user")
	user := v1.User{}
	userRout.POST("/regist", user.RegistByPhone)
	userRout.POST("/login", user.LoginByPwd)
	userRout.POST("/k-login", user.LoginByPhone)

	userAuth := userRout.Use(httpmd.JwtAuth())
	userAuth.GET("/info", user.Info)
	userAuth.POST("/modpass", user.ModPass)
	userAuth.POST("/modinfo", user.ModInfo)
}
