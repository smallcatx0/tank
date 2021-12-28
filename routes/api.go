package routes

import (
	v1 "gtank/controller/v1"

	"github.com/gin-gonic/gin"
)

func registeRoute(router *gin.Engine) {
	router.POST("/login", v1.LoginByPwd)
	router.POST("/mq/dev-null", v1.DevNull)
	router.POST("/mq/push", v1.Push)

}
