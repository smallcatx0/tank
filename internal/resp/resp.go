package resp

import (
	"encoding/json"
	"errors"

	"serv_tpl/internal/global"
	"serv_tpl/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SuccJsonRaw 原样嵌入 data，避免重复解析和序列化；调用方须保证 data 是完整合法的 JSON 值。
func SuccJsonRaw(c *gin.Context, data string) {
	// 步骤1：仅编码请求标识，data 不解析、不校验、不重新编码。
	format := `{"errcode":%d,"msg":"%s","data":%s,"request_id":%s}`
	requestID, _ := json.Marshal(c.GetString(RequestIDKey)) // string 编码不会返回错误。
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.String(200, format, 0, "操作成功", data, requestID)
}

// RequestIDKey 唯一请求id
const RequestIDKey = "x-b3-traceid"

type body struct {
	ErrCode   int         `json:"errcode"`
	Msg       string      `json:"msg"`
	Data      interface{} `json:"data"`
	RequestID string      `json:"request_id"`
}

// Response 统一返回，识别错误链中的业务异常；nil error 或空 *Exception 按通用服务错误处理。
func Response(c *gin.Context, err error) {
	// 步骤1：为空错误保留失败响应，避免调用 Error 或解引用空指针。
	httpCode := 500
	b := body{ErrCode: 50000, Msg: "服务错误"}
	// 步骤2：兼容业务异常的指针和值，以及通过 %w 包装的错误链。
	var e *Exception
	var value Exception
	switch {
	case err == nil:
	case errors.As(err, &e):
		if e == nil {
			break
		}
		httpCode = e.HTTPCode
		b.ErrCode = e.ErrCode
		b.Msg = e.Msg
		b.Data = e.Data
	case errors.As(err, &value):
		httpCode = value.HTTPCode
		b.ErrCode = value.ErrCode
		b.Msg = value.Msg
		b.Data = value.Data
	default:
		if global.Env() == "dev" {
			b.Msg = err.Error()
		} else {
			logger.L().Error(c.Request.RequestURI, zap.Error(err))
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
