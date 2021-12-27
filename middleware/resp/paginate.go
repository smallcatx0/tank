package resp

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	MAX_LIMIT = 1000
)

// 分页规范
type Pagination struct {
	Page      int `json:"page"`
	Limit     int `json:"limit"`
	Total     int `json:"total"`
	TotalPage int `json:"total_page"`
	Offset    int `json:"-"`
}

func NewPagination(page, limit int) *Pagination {
	pg := &Pagination{
		Page:  page,
		Limit: limit,
	}
	if page < 1 {
		pg.Page = 1
	}
	if limit > MAX_LIMIT {
		pg.Limit = MAX_LIMIT
	}
	if limit < 1 {
		pg.Limit = limit
	}
	return pg
}

func NewPage(c *gin.Context) *Pagination {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	return NewPagination(page, limit)
}

func (p *Pagination) Calc(total int) {
	p.Total = total
	p.TotalPage = p.Total / p.Limit
	if p.Total%p.Limit != 0 {
		p.TotalPage = p.TotalPage + 1
	}
	if p.Page > p.TotalPage {
		p.Page = p.TotalPage
	}
	p.Offset = (p.Page - 1) * p.Limit
}

func (p *Pagination) Paginate(tx *gorm.DB) (*gorm.DB, error) {
	var count int64
	err := tx.Count(&count).Error
	if err != nil {
		return tx, err
	}
	p.Calc(int(count))
	return tx.Offset(p.Offset).Limit(p.Limit), nil
}
