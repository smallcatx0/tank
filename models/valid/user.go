package valid

import "serv_tpl/internal/resp"

// UserAdd 是创建用户的请求参数；手机号使用 E.164 格式，如 +8613800138000。
// 仅做输入校验，不承担身份认证、业务授权或数据库唯一性约束。
type UserAdd struct {
	Username string `json:"username" binding:"required,min=1,max=20"`
	Password string `json:"password" binding:"required,min=6,max=20"`
	Email    string `json:"email" binding:"required,email"`
	Phone    string `json:"phone" binding:"required,startswith=+,e164"`
	Status   string `json:"status" binding:"required,oneof=enable disable"`
}

// Valid 检查示例中的保留用户名，不读取或修改请求身份。
func (u *UserAdd) Valid() error {
	if u.Username == "admin" || u.Username == "root" {
		return resp.ParamInValid("用户名不能为admin或root")
	}
	return nil
}
