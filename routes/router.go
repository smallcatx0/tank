package routes

import (
	v1 "gitee.com/smallcatx0/gtank/controller/v1"

	"github.com/gin-gonic/gin"
)

func registeRoute(router *gin.Engine) {
	router.GET("/demo", v1.Demo)
	router.POST("/login", v1.LoginByPwd)
}
