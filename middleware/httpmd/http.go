package httpmd

import (
	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
)

// RequestIDKey 唯一请求id
const RequestIDKey = "request-id"

// SetHeader 设置header
func SetHeader(c *gin.Context) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	requestID := c.GetHeader(RequestIDKey)
	if requestID == "" {
		c.Set(RequestIDKey, uuid.NewV4().String())
	} else {
		c.Set(RequestIDKey, requestID)
	}
}
