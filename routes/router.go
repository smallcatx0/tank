package routes

import (
	v1 "tank/controller/v1"

	"github.com/gin-gonic/gin"
)

func registeRoute(router *gin.Engine) {
	router.GET("/demo", v1.Demo)
}
