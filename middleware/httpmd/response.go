package httpmd

import (
	"net/http"
	"strings"

	"gitee.com/smallcatx0/gtank/pkg/conf"
	"gitee.com/smallcatx0/gtank/pkg/exception"
	glog "gitee.com/smallcatx0/gtank/pkg/glog"

	"github.com/gin-gonic/gin"
)

// Resp 封装响应体
type Resp struct{}

// 响应体
type responseData struct {
	StatusCode uint32      `json:"status_code"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data"`
	RequestID  string      `json:"request_id"`
}

// Succ 成功返回
func (r *Resp) Succ(c *gin.Context, data interface{}, msg ...string) {
	rr := new(responseData)
	rr.StatusCode = http.StatusOK
	if len(msg) == 0 {
		rr.Message = exception.ErrNos[rr.StatusCode]
	} else {
		rr.Message = strings.Join(msg, ",")
	}
	rr.Data = data
	rr.RequestID = c.GetString(RequestIDKey)
	c.JSON(http.StatusOK, &rr)
}

// Fail 失败返回
func (r *Resp) Fail(c *gin.Context, err error) {
	var httpState int
	rr := new(responseData)
	switch e := err.(type) {
	case *exception.Exception:
		rr.StatusCode = e.Code
		rr.Message = e.Msg
		httpState = e.HTTPCode
	default:
		// 记录日志
		if conf.Env() == "dev" {
			rr.Message = err.Error()
		} else {
			glog.Error(c.Request.RequestURI, err.Error())
			rr.Message = "服务错误"
		}
		rr.StatusCode = 400
		httpState = 400
	}
	rr.RequestID = c.GetString(RequestIDKey)
	c.JSON(httpState, &rr)
}

func (r *Resp) SuccJsonRaw(c *gin.Context, data string) {
	format := `{"status_code":%d,"message":"%s","data":%s,"request_id":"%s"}`
	requestId := c.GetString(RequestIDKey)
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.String(200, format, 200, "操作成功", data, requestId)
}
