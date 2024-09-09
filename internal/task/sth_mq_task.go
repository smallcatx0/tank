package task

import (
	"encoding/json"
	"gtank/pkg/glog"
	"math/rand"
	"strings"

	"github.com/adjust/rmq/v5"
	"go.uber.org/zap"
)

type RmqStatus uint8

const (
	RmqStatus_done  RmqStatus = 1 // 成功
	RmqStatus_fail  RmqStatus = 2 // 失败（可重试）
	RmqStatus_error RmqStatus = 3 // 错误（不可重试）
)

const (
	SthMaxRetry uint8 = 5
)

type SthMqTask struct {
	ID     int
	Con    string
	ErrMsg string
	Retry  uint8
	Status RmqStatus
}

func (t *SthMqTask) Serialize() []byte {
	raw, _ := json.Marshal(t)
	return raw
}

func (t *SthMqTask) Unserialize(raw []byte) error {
	return json.Unmarshal(raw, t)
}

func (t *SthMqTask) Run() {
	// 80% 的概率成功
	num := rand.Intn(100)
	if num >= 80 {
		t.Status = RmqStatus_done
	} else {
		t.ErrMsg = "可重试错误"
		t.Status = RmqStatus_fail
		t.Retry += 1
	}

	if t.Retry >= SthMaxRetry {
		t.ErrMsg = "都重试5次了，别重试了"
		t.Status = RmqStatus_error
		// 记录 错误记录
		glog.Error("[sth_mq_task] " + string(t.Serialize()))
	}
}

type SthRetryWorker struct {
	failQ rmq.Queue // 重试队列（二级队列）
}

func (q SthRetryWorker) Consume(dv rmq.Delivery) {
	raw := dv.Payload()
	task := &SthMqTask{}
	err := task.Unserialize([]byte(raw))
	if err != nil {
		// 丢回去下次消费依然反序列化失败，直接记录日志后ACK吧
		glog.D().Z().Error("消费数据失败", zap.String("task_raw", raw))
		err = dv.Ack()
		IfErrLog(err)
		return
	}
	glog.Debug("[sth_mq_task] 开始消费任务: " + raw)
	task.Run()
	if task.Status == RmqStatus_fail {
		err = q.failQ.PublishBytes(task.Serialize())
		IfErrLog(err)
	}
	err = dv.Ack()
	IfErrLog(err)
}

func IfErrLog(err error, msg ...string) {
	if err == nil {
		return
	}
	errMsg := strings.Join(msg, " ")
	errMsg += " err=" + err.Error()
	glog.Error("[sth_mq_task]" + errMsg)
}
