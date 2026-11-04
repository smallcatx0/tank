package schema

import (
	"context"
	"fmt"
	"serv_tpl/internal/resp"

	"gorm.io/gorm"
)

// Shop 定义店铺表模型，状态值与店铺请求参数保持一致。
type Shop struct {
	ID     uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`          // 店铺 ID
	Name   string `gorm:"column:name;type:varchar(50);not null" json:"name"`     // 店铺名称
	Status string `gorm:"column:status;type:varchar(10);not null" json:"status"` // enable 或 disable
}

// TableName 指定店铺表名。
func (Shop) TableName() string { return "shops" }

// Create 在调用方给定的期限内创建店铺；不自动重试，避免不确定提交后的重复写入。
func (s *Shop) Create(ctx context.Context, db *gorm.DB) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("创建店铺: %w", err)
	}
	return nil
}

// ListShops 查询店铺，回填分页总数；Count 与 Find 共用 context，并按 ID 降序。
func ListShops(ctx context.Context, db *gorm.DB, status string, page *resp.Pagination) ([]Shop, error) {
	list := make([]Shop, 0)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 步骤1：先计算筛选后的总数。
	q := db.WithContext(ctx).Model(&Shop{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	q, err := page.Paginate(q)
	if err != nil {
		return nil, fmt.Errorf("统计店铺: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if page.Total == 0 {
		return list, nil
	}
	// 步骤2：继续使用同一期限，不为第二条 SQL 重新计时。
	if err := q.Order("id DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询店铺: %w", err)
	}
	return list, nil
}
