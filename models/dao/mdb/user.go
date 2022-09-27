package mdb

import (
	"database/sql"
	"errors"
	"fmt"
	"gtank/models/dao"
	"strconv"

	"github.com/duke-git/lancet/cryptor"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	User_passsalt = "123123"
)

type User struct {
	Id        int            `gorm:"column:id" json:"id"`
	User      string         `gorm:"column:user" json:"user"`             //账号
	Pass      string         `gorm:"column:pass" json:"pass"`             //密码
	Nickname  string         `gorm:"column:nickname" json:"nickname"`     //昵称
	Truename  string         `gorm:"column:truename" json:"truename"`     //真实姓名
	Phone     string         `gorm:"column:phone" json:"phone"`           //手机号
	Email     string         `gorm:"column:email" json:"email"`           //电子邮箱
	Status    int8           `gorm:"column:status" json:"status"`         //状态
	CreatedAt sql.NullTime   `gorm:"column:created_at" json:"created_at"` //创建时间
	UpdatedAt sql.NullTime   `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at" json:"deleted_at"`
}

func (User) TableName() string {
	return dao.MdbPrefix + "users"
}

func (u *User) GetByPhone() (bool, error) {
	q := dao.MDB.Where("phone=?", u.Phone)
	if u.Id != 0 {
		q = q.Where("id <> ?", u.Id)
	}
	return u.queryOne(q)
}
func (u *User) GetByUser() (bool, error) {
	q := dao.MDB.Where("user=?", u.User)
	if u.Id != 0 {
		q = q.Where("id <> ?", u.Id)
	}
	return u.queryOne(q)
}

func (u *User) queryOne(q *gorm.DB) (bool, error) {
	err := q.First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (u *User) AutoUseName() string {
	// 取手机后四位=>转16进制
	p, _ := strconv.ParseInt(u.Phone[5:], 10, 64)
	id := uuid.NewString()[0:4]
	return fmt.Sprintf("%s_%x", id, p)
}

// 密码加密
func (u *User) passEncry(p string) string {
	return cryptor.Md5String(User_passsalt + p)
}

func (u *User) PassEq(pass string) bool {
	return u.Pass == u.passEncry(pass)
}

func (u *User) SetPass(pass string) {
	u.Pass = u.passEncry(pass)
}
