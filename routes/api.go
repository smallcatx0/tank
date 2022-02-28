package routes

import (
	v1 "gtank/controller/v1"
	"gtank/middleware/httpmd"

	"github.com/gin-gonic/gin"
)

func registeRoute(router *gin.Engine) {
	root := router.Group("/v1")

	userRout := root.Group("/user")
	user := v1.User{}
	userRout.POST("/regist", user.RegistByPhone)
	userRout.POST("/login", user.LoginByPwd)

	userAuth := userRout.Use(httpmd.JwtAuth())
	userAuth.GET("/info", user.Info)
	userAuth.GET("/modpass", user.ModPass)

	demo := v1.Demo{}
	userRout.GET("/userlist", demo.UserList)
}
