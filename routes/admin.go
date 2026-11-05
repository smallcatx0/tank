package routes

import (
	v1 "serv_tpl/controller/v1"
	"serv_tpl/internal/httpmd"

	"github.com/gin-gonic/gin"
)

func registAdmin(r *gin.Engine) {
	root := r.Group("/admin")

	userRout := root.Group("/user").Use(httpmd.JwtAuth())
	u := v1.UserAdmin{}
	userRout.GET("/list", u.List)
}
