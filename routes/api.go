package routes

import (
	v1 "serv_tpl/controller/v1"
	"serv_tpl/internal/httpmd"

	"github.com/gin-gonic/gin"
)

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
