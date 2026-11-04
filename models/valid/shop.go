package valid

import (
	"strings"

	"serv_tpl/internal/resp"
)

// ShopAdd 演示店铺创建参数的标签校验和自定义校验。
type ShopAdd struct {
	Name   string `json:"name" binding:"required,max=50"`
	Status string `json:"status" binding:"required,oneof=enable disable"`
}

// Valid 补充标签规则，拒绝只有空白的店铺名称。
func (p *ShopAdd) Valid() error {
	if strings.TrimSpace(p.Name) == "" {
		return resp.ParamInValid("店铺名称不能全为空白")
	}
	return nil
}
