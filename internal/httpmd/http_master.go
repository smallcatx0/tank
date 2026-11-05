package httpmd

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"serv_tpl/internal/resp"

	"serv_tpl/internal/conf"
	glog "serv_tpl/pkg/glog"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
)

// RequestIDKey 唯一请求id（与 resp.RequestIDKey 同源，包内别名避免重复定义）
const requestIDKey = resp.RequestIDKey

// 日志记录白名单
var LogWrite = map[string]bool{
	"/healthz": true,
	"/ready":   true,
}

// SetHeader 设置header
func SetHeader(c *gin.Context) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	requestID := c.GetHeader(requestIDKey)
	if requestID == "" {
		requestID = uuid.NewV4().String()
		requestID = strings.Replace(requestID, "-", "", -1)
	}
	c.Set(requestIDKey, requestID)
	c.Header(requestIDKey, requestID)
}

// ReqLogFull 记录全量请求日志（包内已有 min_tpl 的 ReqLog，此处保留 master 全量版本并改名）
func ReqLogFull(c *gin.Context) {
	if conf.IsDebug() {
		path := c.Request.RequestURI
		requestData, _ := c.GetRawData()
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestData))
		header, _ := json.Marshal(c.Request.Header)
		if _, ok := LogWrite[path]; ok {
			// 白名单不记录
			return
		}
		glog.Debug("request_log",
			c.GetString(requestIDKey),
			path,
			string(requestData),
			string(header),
		)
	}
}
