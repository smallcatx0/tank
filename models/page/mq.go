package page

import (
	"log"

	request "gitee.com/smallcatx0/gequest"
	"gitee.com/smallcatx0/gtank/models/dao/rdb"
	"gitee.com/smallcatx0/gtank/valid"
	"github.com/gin-gonic/gin"
)

var RdbMq *rdb.Mq
var HttpCli *request.Core

type MqPub struct{}

func (pub *MqPub) Push(c *gin.Context, param *valid.PushParam) error {
	RdbMq := &rdb.Mq{
		Key: "test_key",
	}
	body := &rdb.HttpBody{
		Url:    param.URL,
		Method: "post",
		Body:   param.Body,
		Header: param.Header,
	}
	RdbMq.Push(body)
	return nil
}

type MqHttpSub struct{}

func InitSub() {
	sub := new(MqHttpSub)
	HttpCli = request.New("mq-unifisub", "", 3000)
	sub.goPop(5)
}

func (sub *MqHttpSub) goPop(pool int) {
	for i := 0; i < pool; i++ {
		go func() {
			RdbMq.BPop(httpConsume)
		}()
	}
}

func httpConsume(res string) {
	// 请求 接口
	reqBody := &rdb.HttpBody{}
	reqBody.Build(res)
	log.Print(reqBody)
}
