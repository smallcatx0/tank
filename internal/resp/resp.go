package resp

import (
	"context"
	"encoding/json"
	"errors"
	"serv_tpl/internal/requestctx"

	"serv_tpl/internal/global"
	"serv_tpl/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestIDKey 唯一请求id
const RequestIDKey = "x-b3-traceid"

// SuccJsonRaw 原样嵌入 data，避免重复解析和序列化；调用方须保证 data 是完整合法的 JSON 值。
func SuccJsonRaw(c *gin.Context, data string) {
	// 步骤1：仅编码请求标识，data 不解析、不校验、不重新编码。
	format := `{"errcode":%d,"msg":"%s","data":%s,"request_id":%s}`
	requestID, _ := json.Marshal(c.GetString(RequestIDKey)) // string 编码不会返回错误。
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.String(200, format, 0, "操作成功", data, requestID)
}

type body struct {
	ErrCode   int    `json:"errcode"`
	Msg       string `json:"msg"`
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

// Response 统一返回，识别错误链中的业务异常；nil error 或空 *Exception 按通用服务错误处理。
func Response(c *gin.Context, err error) {
	if c.Request != nil && c.Request.Context().Err() == context.Canceled {
		c.Abort()
		return
	}
	// 步骤1：为空错误保留失败响应，避免调用 Error 或解引用空指针。
	httpCode := 500
	b := body{ErrCode: 50000, Msg: "服务错误"}
	// 步骤2：兼容业务异常的指针形式，以及通过 %w 包装的错误链。
	var e *Exception
	switch {
	case err == nil:
	case errors.Is(err, context.DeadlineExceeded):
		httpCode, b.ErrCode, b.Msg = 504, 50400, "请求处理超时"
		logSystemError(c, err)
	case errors.As(err, &e):
		if e == nil {
			break
		}
		httpCode = e.HTTPCode
		b.ErrCode = e.ErrCode
		b.Msg = e.Msg
		b.Data = e.Data
	default:
		logSystemError(c, err)
		if global.Env() == "dev" {
			b.Msg = err.Error()
		} else {
			b.Msg = "服务错误"
		}
	}
	b.RequestID = c.GetString(RequestIDKey) // 中间件统一写入的值，与 SuccJsonRaw 保持一致
	c.JSON(httpCode, &b)
}

func logSystemError(c *gin.Context, err error) {
	fields := []zap.Field{zap.Error(err), zap.String("operation", c.FullPath()), zap.String("trace_id", c.GetString(RequestIDKey))}
	if c.Request != nil {
		fields = append(fields, zap.Float64("latency_ms", requestctx.LatencyMS(c.Request.Context())))
	}
	logger.L().Error("请求处理失败", fields...)
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
