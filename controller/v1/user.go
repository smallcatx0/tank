package v1

import (
	"serv_tpl/internal/global"
	"serv_tpl/internal/resp"
	"serv_tpl/models/dao/schema"
	"serv_tpl/models/valid"

	"github.com/gin-gonic/gin"
)

// User 演示参数校验和统一响应, 未实现实际业务逻辑
type User struct{}

// Create 演示创建参数校验，不实际创建用户
func (*User) Create(c *gin.Context) {
	// 步骤1：前置格式校验；唯一性和授权仍须在业务处理中保证。
	p := valid.UserAdd{}
	if err := valid.BindJsonAndCheck(c, &p); err != nil {
		resp.Fail(c, err)
		return
	}

	// 步骤2：TODO 接入业务，传递 c.Request.Context()，返回独立响应 DTO。

	resp.Succ(c, gin.H{"demo": true, "validated": true})
}

// List 演示可选筛选和分页响应
func (*User) List(c *gin.Context) {
	// 步骤1：仅本接口使用的筛选保留为局部结构，分页使用统一默认值。
	p := struct {
		Username string `form:"username" binding:"omitempty,min=1,max=20"`
		Phone    string `form:"phone" binding:"omitempty,startswith=+,e164"`
		Status   string `form:"status" binding:"omitempty,oneof=enable disable"`
	}{}
	if err := valid.BindQueryAndCheck(c, &p); err != nil {
		resp.Fail(c, err)
		return
	}

	q := global.MysqlCli.Model(schema.User{})
	if p.Phone != "" {
		q = q.Where("phone = ?", p.Phone)
	}

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
	list := []schema.User{}
	if err := q.Find(&list).Error; err != nil {
		resp.Fail(c, err)
		return
	}
	resp.Paginate(c, pg, list)
}
