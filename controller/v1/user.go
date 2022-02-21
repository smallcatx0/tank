package v1

import (
	"gtank/middleware/resp"
	"gtank/models/dao"
	"gtank/models/dao/mdb"
	"gtank/valid"
	"strconv"
	"time"

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
	t, ok := valid.UserInfo(c)
	if !ok {
		resp.Fail(c, resp.NoLogin)
		return
	}
	type User struct {
		Id        int       `gorm:"column:id" json:"id"`
		User      string    `gorm:"column:user" json:"user"`             //账号            //密码
		Nickname  string    `gorm:"column:nickname" json:"nickname"`     //昵称
		Truename  string    `gorm:"column:truename" json:"truename"`     //真实姓名
		Phone     string    `gorm:"column:phone" json:"phone"`           //手机号
		Email     string    `gorm:"column:email" json:"email"`           //电子邮箱
		Status    int8      `gorm:"column:status" json:"status"`         //状态
		CreatedAt time.Time `gorm:"column:created_at" json:"created_at"` //创建时间
		UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
	}
	u := User{}
	err := dao.MDB.First(&u, t.Uid).Error
	if err != nil {
		resp.Fail(c, err)
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
