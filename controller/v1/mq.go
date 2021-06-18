package v1

import (
	"encoding/json"
	"path/filepath"
	"time"

	"gitee.com/smallcatx0/gtank/models/data"
	"gitee.com/smallcatx0/gtank/pkg/conf"
	"gitee.com/smallcatx0/gtank/pkg/helper"
	"gitee.com/smallcatx0/gtank/valid"
	"github.com/gin-gonic/gin"
)

func Push(c *gin.Context) {
	param := valid.PushParam{}
	err := valid.BindAndCheck(c, &param)
	if err != nil {
		r.Fail(c, err)
		return
	}
	// 丢消息
	err = new(data.MqPub).Push(c, &param)
	if err != nil {
		r.Fail(c, err)
		return
	}
	r.Succ(c, param)
}

// 将请求全量记录
func DevNull(c *gin.Context) {
	requesBody, _ := c.GetRawData()
	param := make(map[string]interface{}, 10)
	param["time"] = time.Now().String()
	param["body"] = string(requesBody)
	param["header"] = c.Request.Header
	content, _ := json.Marshal(param)
	content = append(content, 10)

	f := filepath.Dir(conf.AppConf.GetString("log.path")) + "/MqRecv.txt"
	helper.TouchDir(f)
	err := helper.AppendFile(f, content, 0777)
	if err != nil {
		r.Fail(c, err)
		return
	}
	r.Succ(c, param)
}
