package v1

import (
	"gtank/middleware/resp"
	"gtank/models/dao"
	"gtank/models/dao/mdb"
	"gtank/valid"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type User struct{}

// 手机号注册
func (User) RegistByPhone(c *gin.Context) {
	param := valid.PhoneReg{}
	err := valid.BindJsonAndCheck(c, &param)
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
	token, err := grantToken(*u)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.Succ(c, map[string]interface{}{
		"auth": token,
	})
}

func grantToken(u mdb.User) (string, error) {
	j := &valid.JWTData{
		Uid:   u.Id,
		User:  u.User,
		Phone: u.Phone,
	}
	return j.Generate()
}

// 手机号登录
func (User) LoginByPhone(c *gin.Context) {
	param := valid.PhoneReg{}
	err := valid.BindJsonAndCheck(c, &param)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	u := &mdb.User{
		Phone: param.Phone,
	}
	exist, err := u.GetByPhone()
	if err != nil {
		resp.Fail(c, err)
		return
	}
	if !exist {
		resp.Fail(c, resp.ParamInValid("未注册"))
		return
	}
	token, err := grantToken(*u)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.Succ(c, map[string]interface{}{
		"auth": token,
	})
}

// 用户名密码登录
func (User) LoginByPwd(c *gin.Context) {
	p := valid.UserLogin{}
	err := valid.BindJsonAndCheck(c, &p)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	u := &mdb.User{User: p.User}
	ok, err := u.GetByUser()
	if err != nil {
		resp.Fail(c, err)
		return
	}
	if !ok {
		resp.Fail(c, resp.ParamInValid("用户名或密码错误"))
		return
	}
	if !u.PassEq(p.Pass) {
		resp.Fail(c, resp.ParamInValid("用户名或密码错误"))
		return
	}
	token, err := grantToken(*u)
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
		return
	}
	resp.Succ(c, u)
}

// 修改密码
func (User) ModPass(c *gin.Context) {
	param := valid.ModPass{}
	err := valid.BindJsonAndCheck(c, &param)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	t, ok := valid.UserInfo(c)
	if !ok {
		resp.Fail(c, resp.NoLogin)
		return
	}
	u := &mdb.User{}
	err = dao.MDB.First(u, t.Uid).Error
	if err != nil {
		resp.Fail(c, err)
		return
	}
	switch strings.ToLower(param.Type) {
	case "phone":
		if u.Phone != param.Phone {
			resp.Fail(c, resp.Illegal)
			return
		}
	case "pass":
		// 数据库查询旧密码对比
		if u.Pass != "" && !u.PassEq(param.OldPass) {
			resp.Fail(c, resp.ParamInValid("密码错误"))
			return
		}
	case "email":
		if u.Email != param.Email {
			resp.Fail(c, resp.Illegal)
			return
		}
	}
	u.SetPass(param.Pass)
	err = dao.MDB.Select("pass").Updates(u).Error
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.Succ(c, nil)
}

// 编辑基础信息
func (User) ModInfo(c *gin.Context) {
	t, ok := valid.UserInfo(c)
	if !ok {
		resp.Fail(c, resp.NoLogin)
		return
	}
	p := valid.UserInfoModParam{}
	err := valid.BindJsonAndCheck(c, &p)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	u := &mdb.User{}
	// 判断user是否存在
	if p.User != "" {
		u.Id = t.Uid
		u.User = p.User
		ok, err := u.GetByUser()
		if err != nil {
			resp.Fail(c, err)
			return
		}
		if ok {
			resp.Fail(c, resp.ParamInValid("用户名已存在"))
			return
		}
	}
	err = dao.MDB.First(u, t.Uid).Error
	if err != nil {
		resp.Fail(c, err)
		return
	}
	if p.Nickname != "" {
		u.Nickname = p.Nickname
	}
	if p.Truename != "" {
		u.Truename = p.Truename
	}
	if p.User != "" {
		u.User = p.User
	}
	res := dao.MDB.
		Select("nickname", "truename", "user").
		Updates(u)
	if res.Error != nil {
		resp.Fail(c, res.Error)
		return
	}
	resp.Succ(c, map[string]interface{}{
		"col": res.RowsAffected,
	})
}
