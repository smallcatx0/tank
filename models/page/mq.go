package page

import (
	"gitee.com/smallcatx0/gtank/models/dao/rdb"
	"gitee.com/smallcatx0/gtank/valid"
	"github.com/gin-gonic/gin"
)

type MqPub struct{}

func (pub *MqPub) Push(c *gin.Context, param *valid.PushParam) error {

	mq := rdb.Mq{
		Key: "test_key",
	}
	body := &rdb.HttpBody{
		Url:    param.URL,
		Method: "post",
		Body:   param.Body,
		Header: param.Header,
	}
	mq.Push(body)
}
