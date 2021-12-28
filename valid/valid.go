package valid

import (
	"github.com/gin-gonic/gin"
	"gtank/middleware/resp"
)

type CustomValidor interface {
	Valid() error
}

func BindAndCheck(c *gin.Context, param interface{}) error {
	err := c.ShouldBindJSON(param)
	if err != nil {
		return resp.ParamInValid("json解析失败 " + err.Error())
	}
	// 自定义验证规则
	if validor, ok := param.(CustomValidor); ok {
		err = validor.Valid()
		if err != nil {
			return resp.ParamInValid(err.Error())
		}
	}
	return nil
}
