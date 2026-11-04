package valid

import (
	"serv_tpl/internal/resp"

	"github.com/gin-gonic/gin"
)

type UserAdd struct {
	Username string `json:"username" binding:"required,min=1,max=20"`
	Password string `json:"password" binding:"required,min=6,max=20"`
	Email    string `json:"email" binding:"required,email"`
	Phone    string `json:"phone" binding:"required,phone"`
	Role     string `json:"role" binding:"required,oneof=admin user"`
	Status   string `json:"status" binding:"required,oneof=enable disable"`

	// 通过复合逻辑获取的字段,不从json中获取
	OptUserID string `json:"-"`
}

var canNotUserName = map[string]bool{
	"admin": true,
	"root":  true,
}

// 所有参数校验逻辑,都应该前置
func (u *UserAdd) Valid(c *gin.Context) error {
	// 处理一些 复合的验证逻辑
	if canNotUserName[u.Username] {
		return resp.ParamInValid("用户名不能为admin或root")
	}

	// 通过复合逻辑获取的字段,如: 通过token操作者信息
	u.OptUserID = c.GetHeader("X-User-Id")

	return nil
}
