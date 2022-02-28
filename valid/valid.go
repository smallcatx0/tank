package valid

import (
	"gtank/middleware/resp"

	"github.com/gin-gonic/gin"
)

type CustomValidor interface {
	Valid() error
}

func BindJsonAndCheck(c *gin.Context, param interface{}) error {
	err := c.ShouldBindJSON(param)
	if err != nil {
		return resp.ParamInValid("json解析失败 " + err.Error())
	}
	// 自定义验证规则
	if validor, ok := param.(CustomValidor); ok {
		err = validor.Valid()
	}
	return err
}
