package rdb

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"serv_tpl/internal/global"
	"serv_tpl/pkg/logger"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
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
	res := global.RedisCli.LPush(context.Background(), mq.Key, msg.String())
	if err := res.Err(); err != nil {
		logger.L().Error("PushQueue err", zap.String("err", err.Error()))
	}
}

// 消费者，常驻内存
func (mq *Mq) BPop(hander func(string)) {
	for {
		// 阻塞式监听该key
		res := global.RedisCli.BRPop(context.Background(), time.Second*10, mq.Key)
		err := res.Err()
		if err == nil {
			hander(res.Val()[1])
		}
		if err == redis.Nil {
			log.Print("queueIsEmpty")
		}
	}
}
