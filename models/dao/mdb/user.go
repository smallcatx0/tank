package mdb

import (
	"database/sql"

	"gorm.io/gorm"
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
	return "users"
}
