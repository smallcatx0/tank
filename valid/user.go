package valid

import (
	"gtank/middleware/resp"
)

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

type UserModify struct {
	User     string `json:"user"`
	Pass     string `json:"pass"`
	Nickname string `json:"nickname"`
	Truename string `json:"truename"`
	Email    string `json:"email"`
}
