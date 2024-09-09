package rdb

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"gtank/models/dao"
	"gtank/pkg/glog"

	"github.com/redis/go-redis/v9"
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

func (mq *Mq) Push(msg MqMsg) {
	res := dao.RedisCli.LPush(context.Background(), mq.Key, msg.String())
	if err := res.Err(); err != nil {
		glog.Error("PushQueue err", "", err.Error())
	}
}

// 消费者，常驻内存
func (mq *Mq) BPop(hander func(string)) {
	for {
		// 阻塞式监听该key
		res := dao.RedisCli.BRPop(context.Background(), time.Second*10, mq.Key)
		err := res.Err()
		if err == nil {
			hander(res.Val()[1])
		}
		if err == redis.Nil {
			log.Print("queueIsEmpty")
		}
	}
}
