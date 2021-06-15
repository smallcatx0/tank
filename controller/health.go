package controller

import (
	"gitee.com/smallcatx0/gtank/bootstrap"
	"gitee.com/smallcatx0/gtank/middleware/httpmd"
	"gitee.com/smallcatx0/gtank/pkg/conf"
	"gitee.com/smallcatx0/gtank/pkg/exception"
	"gitee.com/smallcatx0/gtank/pkg/glog"

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
	config := conf.AppConf
	ala := glog.DingAlarmNew(
		config.GetString("dingRobot.webHook"),
		config.GetString("dingRobot.robot"),
	)
	// ala.Markdown("markdown", "### 标题三 \n\n内容").AtPhones("18681636749").Send()
	// ala.Text("文本测试内容").AtPhones("18681636749").Send()
	msg := glog.DingMsg{Msgtype: "markdown"}
	msg.Markdown.Title = "testMard"
	msg.Markdown.Text = "### 标题三 \n\n内容"
	msg.At.AtMobiles = []string{"18681636749"}
	ala.SendMsg(&msg)
	r.SuccJsonRaw(c, "{\"id\":1,\"weight\":100}")
}

// ReloadConf 重新加载配置文件
func ReloadConf(c *gin.Context) {
	bootstrap.InitConf(&bootstrap.Param.C)
	bootstrap.InitLog()
	bootstrap.InitDB()
	r.Succ(c, "成功")
}
