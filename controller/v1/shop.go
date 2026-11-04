package v1

import (
	"serv_tpl/internal/global"
	"serv_tpl/internal/resp"
	"serv_tpl/models/dao/schema"
	"serv_tpl/models/valid"

	"github.com/gin-gonic/gin"
)

// Shop 演示参数验证、错误响应及分页，不执行数据库读写。
type Shop struct{}

// Create 校验店铺参数并返回演示结果，不实际创建店铺。
func (*Shop) Create(c *gin.Context) {
	// 步骤1：绑定 JSON，并执行标签及自定义校验。
	p := valid.ShopAdd{}
	if err := valid.BindJsonAndCheck(c, &p); err != nil {
		resp.Fail(c, err)
		return
	}
	// 步骤2：实际接入时在此调用创建业务。
	resp.Succ(c, gin.H{"demo": true, "validated": true})
}

// List 使用固定店铺数据演示筛选与分页响应。
func (*Shop) List(c *gin.Context) {
	// 步骤1：绑定可选筛选条件，参数错误统一返回。
	p := struct {
		Status string `form:"status" binding:"omitempty,oneof=enable disable"`
	}{}
	if err := valid.BindQueryAndCheck(c, &p); err != nil {
		resp.Fail(c, err)
		return
	}

	// 步骤2：准备查询条件。
	q := global.MysqlCli.Table("shops")
	if p.Status != "" {
		q = q.Where("status = ?", p.Status)
	}

	// 步骤3：计算分页并返回统一结构。
	pg := resp.NewPage(c)
	q, err := pg.Paginate(q)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	if pg.Total == 0 {
		resp.Paginate(c, pg, nil)
		return
	}
	list := make([]schema.Shop, 0, pg.Limit)
	if err := q.Find(&list).Error; err != nil {
		resp.Fail(c, err)
		return
	}
	resp.Paginate(c, pg, list)
}
