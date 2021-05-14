package mdb

import (
	"database/sql"

	"gorm.io/gorm"
)

type Users struct {
	ID        uint64         `gorm:"column:id;primary_key" json:"id"`
	Username  string         `gorm:"column:username" json:"username"`     // 用户名
	Password  string         `gorm:"column:password" json:"password"`     // 密码
	Nickname  string         `gorm:"column:nickname" json:"nickname"`     // 昵称
	Phone     string         `gorm:"column:phone" json:"phone"`           // 手机号
	Email     string         `gorm:"column:email" json:"email"`           // 邮箱
	Status    uint64         `gorm:"column:status" json:"status"`         // 状态
	CreatedAt sql.NullTime   `gorm:"column:created_at" json:"created_at"` // 创建日期
	UpdatedAt sql.NullTime   `gorm:"column:updated_at" json:"-"`          // 更新日期
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at" json:"deleted_at"` // 软删除
}

func (u *Users) TableName() string {
	return "users"
}
