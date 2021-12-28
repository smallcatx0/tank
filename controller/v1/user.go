package v1

import (
	"gtank/middleware/resp"
	"gtank/valid"

	"github.com/gin-gonic/gin"
)

type User struct{}

// 手机号注册
func (User) RegistByPhone(c *gin.Context) {

}

// 用户名密码登录
func (User) LoginByPwd(c *gin.Context) {
	param := valid.UserLogin{}
	err := valid.BindAndCheck(c, &param)
	if err != nil {
		resp.Fail(c, err)
		return
	}
}

func (User) Info(c *gin.Context) {

}
