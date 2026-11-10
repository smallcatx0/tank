package httpmd

import (
	"net/http"
	"serv_tpl/internal/requestctx"
	"serv_tpl/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 在请求边界恢复 panic，禁止输出请求原文和 panic 值。
func Recovery(c *gin.Context) {
	defer func() {
		if recover() != nil {
			c.Set(panicKey, true)
			logger.L().Error("HTTP 请求 panic",
				zap.String("component", "http"),
				zap.String("route", c.FullPath()),
				zap.String("trace_id", c.GetString(RequestIDKey)),
				zap.Float64("latency_ms", requestctx.LatencyMS(c.Request.Context())),
				zap.String("span_id", requestctx.SpanID(c.Request.Context())),
				zap.Stack("stack"),
			)
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"errcode": 50000, "msg": "服务错误", "data": nil, "request_id": c.GetString(RequestIDKey)})
			} else {
				c.Abort()
			}
		}
	}()
	c.Next()
}
