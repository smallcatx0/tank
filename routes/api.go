package routes

import (
	v1 "gtank/controller/v1"

	"github.com/gin-gonic/gin"
)

func registeRoute(router *gin.Engine) {
	root := router.Group("/v1")

	userRout := root.Group("/user")
	user := v1.User{}
	userRout.POST("/regist", user.RegistByPhone)
	userRout.POST("/modify", user.Modify)
	userRout.POST("/login", user.LoginByPwd)

	demo := v1.Demo{}
	userRout.GET("/userlist", demo.UserList)
}
