package v1

import (
	"gtank/middleware/resp"
	"gtank/models/dao"
	"gtank/models/dao/mdb"
	"gtank/valid"
	"strconv"

	"github.com/gin-gonic/gin"
)

type User struct{}

// 手机号注册
func (User) RegistByPhone(c *gin.Context) {
	param := struct {
		Phone string `json:"phone" binding:"required"`
	}{}
	err := valid.BindAndCheck(c, &param)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	// 先判断手机号是否存在
	u := &mdb.User{
		Phone: param.Phone,
	}
	exist, err := u.GetByPhone()
	if err != nil {
		resp.Fail(c, err)
		return
	}
	if exist {
		resp.Fail(c, resp.ParamInValid("手机号已经存在"))
		return
	}
	u = &mdb.User{
		Phone: param.Phone,
	}
	u.User = u.AutoUseName() // 自动生成用户名
	err = dao.MDB.Create(u).Error
	if err != nil {
		resp.Fail(c, err)
		return
	}
	j := &valid.JWTData{
		Uid:   strconv.Itoa(u.Id),
		User:  u.User,
		Phone: u.Phone,
	}
	token, err := j.Generate()
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.Succ(c, map[string]interface{}{
		"auth": token,
	})
}

// 查看基本信息
func (User) Info(c *gin.Context) {
	u, ok := valid.GetUserInfo(c)
	if !ok {
		resp.Fail(c, resp.NoLogin)
	}
	resp.Succ(c, u)
}

// 修改基本信息
func (User) Modify(c *gin.Context) {

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
