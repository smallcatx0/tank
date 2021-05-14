package controller

import (
	"gitee.com/smallcatx0/gtank/bootstrap"
	"gitee.com/smallcatx0/gtank/middleware/httpmd"
	"gitee.com/smallcatx0/gtank/pkg/exception"
	glog "gitee.com/smallcatx0/gtank/pkg/glog"

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
	glog.Info("INFO logs", c.GetString(httpmd.RequestIDKey), "{\"id\":1,\"weight\":100}")
	ala := glog.DingAlarmNew("https://oapi.dingtalk.com/robot/send?access_token=90526e10d036265881023da81c1740240a4ac3434954810de42319d074b841ac", "SECfa8c17407ea9d632eef8c09e6ad205049b95c7beb8b809f4298af306460f1d23")
	msg := glog.DingBody{
		Msgtype: "text",
		Text: glog.DingBodyText{
			Content: "golang Ding",
		},
	}

	ala.Send(msg)
	r.SuccJsonRaw(c, "{\"id\":1,\"weight\":100}")
}

// ReloadConf 重新加载配置文件
func ReloadConf(c *gin.Context) {
	bootstrap.InitConf(&bootstrap.Param.C)
	bootstrap.InitLog()
	bootstrap.InitDB()
	r.Succ(c, "成功")
}
