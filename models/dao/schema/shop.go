package schema

// Shop 定义店铺表模型，状态值与店铺请求参数保持一致。
type Shop struct {
	ID     uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`          // 店铺 ID
	Name   string `gorm:"column:name;type:varchar(50);not null" json:"name"`     // 店铺名称
	Status string `gorm:"column:status;type:varchar(10);not null" json:"status"` // enable 或 disable
}

// TableName 指定店铺表名。
func (Shop) TableName() string { return "shops" }
