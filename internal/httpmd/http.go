package httpmd

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"serv_tpl/internal/requestctx"
	"serv_tpl/internal/resp"
	"serv_tpl/pkg/logger"
)

// RequestIDKey 是 Gin 上下文中的请求标识键。
const RequestIDKey = resp.RequestIDKey

// ReqLog 记录请求完成情况，不读取请求体或完整 URI；须装配于 Recovery 之前。
func ReqLog(c *gin.Context) {
	// 步骤1：等待业务与 Recovery 完成后记录最终状态。
	c.Next()
	ctx := c.Request.Context()
	logger.L().Info("HTTP 请求完成", zap.String("method", c.Request.Method),
		zap.String("route", c.FullPath()), zap.Int("status", c.Writer.Status()),
		zap.String("trace_id", requestctx.TraceID(ctx)), zap.Float64("latency_ms", requestctx.LatencyMS(ctx)))
}
