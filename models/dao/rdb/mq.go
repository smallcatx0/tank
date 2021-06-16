package rdb

import (
	"encoding/json"
	"time"

	"gitee.com/smallcatx0/gtank/models/dao"
	"gitee.com/smallcatx0/gtank/pkg/glog"
)

type Mq struct {
	Key string
}

type MqMsg interface {
	String() string
	Build(string) error
}

type HttpBody struct {
	Url       string
	Method    string
	Body      string
	Header    string
	RequestId string
}

func (b *HttpBody) String() string {
	jsonstr, _ := json.Marshal(b)
	return string(jsonstr)
}

func (b *HttpBody) Build(jsonStr string) (err error) {
	return json.Unmarshal([]byte(jsonStr), b)
}

func (mq *Mq) Push(msg *MqMsg) {
	res := dao.Rdb.LPush(dao.Rdb.Context(), mq.Key, (*msg).String())
	if err := res.Err(); err != nil {
		glog.Error("PushQueue err", "", err.Error())
	}
}

// 消费者，常驻内存
func (mq *Mq) BPop(hander func(string)) {
	for {
		// 阻塞式监听该key
		res := dao.Rdb.BRPop(dao.Rdb.Context(), time.Second, mq.Key)
		if err := res.Err(); err != nil {
			// 如果是超时，继续监听
		}
		hander(res.String())
	}
}
