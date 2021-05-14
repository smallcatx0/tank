package v1

import (
	"gitee.com/smallcatx0/gtank/models/page"
	"github.com/gin-gonic/gin"
)

type LoginByPwdParam struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func LoginByPwd(c *gin.Context) {
	var param LoginByPwdParam
	err := c.ShouldBindJSON(&param)
	if err != nil {
		r.Fail(c, err)
	}
	res := new(page.User).LoginByPwd()
	r.Succ(c, res)
}
