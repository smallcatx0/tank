package valid

import (
	"gtank/middleware/resp"
)

type UserRegParam struct {
	Phone string `json:"phone" binding:"required"`
}

type UserLogin struct {
	User string `json:"user" binding:"required"`
	Pass string `json:"pass" binding:"required"`
}

func (u *UserLogin) Valid() error {
	if u.User == "admin" {
		return resp.ParamInValid("不允许使用该用户名")
	}
	return nil
}
