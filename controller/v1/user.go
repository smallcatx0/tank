package v1

import (
	"serv_tpl/internal/resp"
	"serv_tpl/models/valid"

	"github.com/gin-gonic/gin"
)

// CRUD demo
type User struct{}

func (*User) Create(c *gin.Context) {
	p := valid.UserAdd{}
	err := valid.BindJsonAndCheck(c, &p)
	if err != nil {
		resp.Fail(c, err)
		return
	}

	// TODO: 逻辑处理

	resp.Succ(c, p)
}

func (*User) List(c *gin.Context) {
	p := struct { // 不被多次引用的,直接使用 匿名结构体
		Username string `form:"username" binding:"required,min=1,max=20"`
		Phone    string `form:"phone" binding:"required,phone"`
		Role     string `form:"role" binding:"required,oneof=admin user"`
		Status   string `form:"status" binding:"required,oneof=enable disable"`
	}{}
	err := valid.BindQueryAndCheck(c, &p)
	if err != nil {
		resp.Fail(c, err)
		return
	}

	// TODO: 逻辑处理

	resp.Succ(c, p)
}
