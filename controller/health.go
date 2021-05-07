package controller

import (
	"gitee.com/smallcatx0/gtank/bootstrap"
	"gitee.com/smallcatx0/gtank/middleware/httpmd"
	"gitee.com/smallcatx0/gtank/pkg/exception"

	"github.com/gin-gonic/gin"
)

var r = new(httpmd.Resp)

func Healthz(c *gin.Context) {
	r.Succ(c, "")
}

func Ready(c *gin.Context) {
	r.Succ(c, exception.ErrNos[200])
}

func Test(c *gin.Context) {
	r.SuccJsonRaw(c, "{\"id\":1,\"weight\":100}")
}

// ReloadConf 重新加载配置文件
func ReloadConf(c *gin.Context) {
	bootstrap.InitConf(&bootstrap.Param.C)
	bootstrap.InitLog()
	bootstrap.InitDB()
	r.Succ(c, "成功")
}
