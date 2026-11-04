package response

import (
	"serv_tpl/internal/global"
	"serv_tpl/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func SuccJsonRaw(c *gin.Context, data string) {
	format := `{"errcode":%d,"msg":"%s","data":%s,"request_id":"%s"}`
	requestId := c.GetString(RequestIDKey)
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.String(200, format, 200, "操作成功", data, requestId)
}

// RequestIDKey 唯一请求id
const RequestIDKey = "x-b3-traceid"

type body struct {
	ErrCode   int         `json:"errcode"`
	Msg       string      `json:"msg"`
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id"`
}

// Response 统一返回
func Response(c *gin.Context, err error) {
	var httpCode int
	b := body{}
	switch e := err.(type) {
	case *Exception:
		httpCode = e.HTTPCode
		b.ErrCode = e.ErrCode
		b.Msg = e.Msg
		b.Data = e.Data
	case Exception:
		httpCode = e.HTTPCode
		b.ErrCode = e.ErrCode
		b.Msg = e.Msg
		b.Data = e.Data
	default:
		httpCode = 500
		b.ErrCode = 50000
		if global.Env() == "dev" {
			b.Msg = e.Error()
		} else {
			logger.L().Error(c.Request.RequestURI, zap.Error(e))
			b.Msg = "服务错误"
		}
	}
	b.RequestID = c.GetString(RequestIDKey) // 中间件统一写入的值，与 SuccJsonRaw 保持一致
	c.JSON(httpCode, &b)
}

func Succ(c *gin.Context, data any) {
	Response(c, NewSucc(data))
}

func Fail(c *gin.Context, err error) {
	Response(c, err)
}

func Paginate(c *gin.Context, pg *Pagination, list any) {
	Succ(c, map[string]any{
		"page": pg,
		"list": list,
	})
}
