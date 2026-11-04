package valid

import (
	"github.com/gin-gonic/gin"
	"serv_tpl/internal/resp"
)

// CustomValidor 补充标签校验之外的输入规则，不依赖 HTTP 身份上下文。
type CustomValidor interface {
	Valid() error
}

// BindJsonAndCheck 绑定 JSON 并执行输入校验，对外隐藏底层解析细节。
func BindJsonAndCheck(c *gin.Context, param interface{}) error {
	// 步骤1：绑定及标签校验失败时仅返回安全的外部提示。
	if err := c.ShouldBindJSON(param); err != nil {
		return resp.ParamInValid("JSON 请求参数无效")
	}
	// 步骤2：执行不依赖传输层的自定义校验。
	if v, ok := param.(CustomValidor); ok {
		return v.Valid()
	}
	return nil
}

// BindQueryAndCheck 绑定查询参数并校验，对外隐藏底层解析细节。
func BindQueryAndCheck(c *gin.Context, param interface{}) error {
	// 步骤1：绑定及标签校验失败时返回查询参数提示。
	if err := c.ShouldBindQuery(param); err != nil {
		return resp.ParamInValid("查询参数无效")
	}
	// 步骤2：执行自定义输入校验。
	if v, ok := param.(CustomValidor); ok {
		return v.Valid()
	}
	return nil
}
