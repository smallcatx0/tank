// Package httpmd 提供 HTTP 请求链路的通用中间件（链路标识、访问日志等），
// 仅由 cmd / routes 装配使用。
package httpmd

import (
	"strings"

	uuid "github.com/satori/go.uuid"

	"github.com/gin-gonic/gin"
)

// RequestIDHeader 是调用方传入请求标识的 HTTP 请求头名称。
const RequestIDHeader = "x-b3-traceid"

// Trace 将请求头中的标识写入 Gin 上下文，供响应和日志关联；缺失时保留空字符串。
func Trace(c *gin.Context) {
	// 步骤1：在业务处理及错误恢复之前写入本次请求标识。
	requestID := c.GetHeader(RequestIDHeader)
	if requestID == "" {
		requestID = uuid.NewV4().String()
		requestID = strings.ReplaceAll(requestID, "-", "")
	}
	c.Set(RequestIDKey, requestID)
	c.Header(RequestIDKey, requestID)
	c.Next()
}
