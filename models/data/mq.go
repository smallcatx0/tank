package data

import (
	"encoding/json"
	"time"

	request "gitee.com/smallcatx0/gequest"
	"gitee.com/smallcatx0/gtank/models/dao/rdb"
	"gitee.com/smallcatx0/gtank/valid"
	"github.com/gin-gonic/gin"
)

var RdbMq *rdb.Mq
var HttpCli *request.Core

type MqPub struct{}

func (pub *MqPub) Push(c *gin.Context, param *valid.PushParam) error {
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

func InitSub(pool int) {
	RdbMq = &rdb.Mq{
		Key: "test_key",
	}
	HttpCli = request.New("mq-unifisub", "", 3000).Debug(true)
	new(MqHttpSub).goPop(pool)
}

func (sub *MqHttpSub) goPop(pool int) {
	for i := 0; i < pool; i++ {
		go func() {
			RdbMq.BPop(httpConsume)
		}()
	}
}

// httpConsume http消费者
func httpConsume(res string) {
	// 请求 接口
	reqBody := &rdb.HttpBody{}
	reqBody.Build(res)

	hd := make(map[string]string, 5)
	json.Unmarshal([]byte(reqBody.Header), &hd)

	// TODO:每两秒，消费一条
	time.Sleep(time.Second * 2)
	HttpCli.SetMethod(reqBody.Method).
		SetUri(reqBody.Url).
		SetBody([]byte(reqBody.Body)).
		SetHeaders(hd).
		Send()
}