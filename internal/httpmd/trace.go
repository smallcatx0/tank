// Package httpmd 提供 HTTP 请求链路的通用中间件（链路标识、访问日志等），
// 仅由 cmd / routes 装配使用。
package httpmd

import (
	"encoding/hex"
	"serv_tpl/internal/requestctx"
	"strings"

	uuid "github.com/satori/go.uuid"

	"github.com/gin-gonic/gin"
)

// RequestIDHeader 是调用方传入请求标识的 HTTP 请求头名称。
const RequestIDHeader = "x-b3-traceid"

// Trace 优先使用有效追踪上下文，否则校验 B3；同步 Gin、响应头及耗时起点。
func Trace(c *gin.Context) {
	// 步骤1：在业务处理及错误恢复之前写入本次请求标识。
	requestID := requestctx.TraceID(c.Request.Context())
	if requestID == "" {
		requestID = c.GetHeader(RequestIDHeader)
	}
	if !validTraceID(requestID) {
		requestID = uuid.NewV4().String()
		requestID = strings.ReplaceAll(requestID, "-", "")
	}
	requestID = strings.ToLower(requestID)
	c.Request = c.Request.WithContext(requestctx.WithTrace(c.Request.Context(), requestID))
	c.Set(RequestIDKey, requestID)
	c.Header(RequestIDKey, requestID)
	c.Next()
}

func validTraceID(id string) bool {
	if len(id) != 16 && len(id) != 32 {
		return false
	}
	if _, err := hex.DecodeString(id); err != nil {
		return false
	}
	return strings.Trim(id, "0") != ""
}
