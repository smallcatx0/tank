package httpmd

import (
	"bytes"
	"encoding/json"
	"io"
	"serv_tpl/internal/global"
	"serv_tpl/internal/resp"
	"serv_tpl/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestIDKey 唯一请求id
const RequestIDKey = resp.RequestIDKey

// 日志记录白名单
var LogWrite = map[string]bool{
	"/healthz": true,
	"/ready":   true,
}

// ReqLog 记录全量请求日志
func ReqLog(c *gin.Context) {
	conf := global.AppConf
	debug := conf.GetBool("http.debug")
	if debug {
		path := c.Request.RequestURI
		requestData, _ := c.GetRawData()
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestData))
		header, _ := json.Marshal(c.Request.Header)
		if _, ok := LogWrite[path]; ok {
			// 白名单不记录
			return
		}
		logger.L().Debug("request_log",
			zap.String("request_id", c.GetString(RequestIDKey)),
			zap.String("path", path),
			zap.String("request_data", string(requestData)),
			zap.String("header", string(header)),
		)
	}

}
