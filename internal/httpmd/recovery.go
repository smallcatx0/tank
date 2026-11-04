package httpmd

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"serv_tpl/pkg/logger"
)

// Recovery 在请求边界恢复 panic，禁止输出请求原文和 panic 值。
func Recovery(c *gin.Context) {
	defer func() {
		if recover() != nil {
			logger.L().Error("HTTP 请求 panic", zap.String("component", "http"), zap.String("route", c.FullPath()), zap.Stack("stack"))
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"errcode": 50000, "msg": "服务错误", "data": nil, "request_id": c.GetString(RequestIDKey)})
			} else {
				c.Abort()
			}
		}
	}()
	c.Next()
}
