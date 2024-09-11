package sthjob

import (
	"fmt"
	"time"

	"github.com/adjust/rmq/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RmqJob struct {
	redisCli *redis.Client // redis 客户端

	conn         rmq.Connection // rmq的链接
	q            rmq.Queue      // 队列实例
	errCh        chan error     // rmq 错误信息
	consumerIns  rmq.Consumer   // 消费者实例
	name         string         // 队列名称
	consumerTags []string       // 消费者tags
	Logger       *zap.Logger    // 日志实例
	rate         int            // 队列消费频率 (每秒从redis中拿多少条数据到本地)
	//
	UnAckedCleanD time.Duration // 未ack任务清理间隔 默认1分钟
}

func NewRmqJob(cli *redis.Client, name string) (*RmqJob, error) {
	j := RmqJob{
		name:          name,
		redisCli:      cli,
		Logger:        zap.L(), // 默认使用 zap 全局logger
		errCh:         make(chan error, 10),
		UnAckedCleanD: time.Minute,
	}
	var err error
	connTag := Hostname() + "_" + j.name
	j.conn, err = rmq.OpenConnectionWithRedisClient(connTag, cli, j.errCh)
	if err != nil {
		return nil, err
	}
	j.q, err = j.conn.OpenQueue(j.name)
	if err != nil {
		return nil, err
	}
	return &j, nil
}
func (j *RmqJob) Queue() rmq.Queue {
	return j.q
}

func (j *RmqJob) Start(workers []rmq.Consumer, rate int64) error {
	var err error
	j.consumerIns = workers[0]
	j.consumerTags = make([]string, len(workers))
	err = j.q.StartConsuming(rate, time.Second)
	if err != nil {
		j.Logger.Error(err.Error())
		return err
	}
	for i := 0; i < len(workers); i++ {
		workerTag := fmt.Sprintf("%s_%s#%d",
			Hostname(), j.name, i,
		)
		j.consumerTags[i], err = j.q.AddConsumer(workerTag, workers[i])
		if err != nil {
			j.Logger.Error(err.Error())
			return err
		}
	}
	j.rate = int(rate)

	go j.clearUnAcked() // 5分钟清理一次未ack的任务
	return nil
}

// 清理未应答的任务
func (j *RmqJob) clearUnAcked() {
	cleaner := rmq.NewCleaner(j.conn)
	ticker := time.NewTicker(j.UnAckedCleanD)
	defer ticker.Stop()
	var count int64
	var err error
	for {
		<-ticker.C
		count, err = cleaner.Clean()
		if err != nil {
			j.Logger.Error(err.Error())
		} else {
			j.Logger.Info(
				fmt.Sprintf("清理未ack任务，queue=%s, count=%d", j.name, count),
			)
		}
	}
}

func (j *RmqJob) MeticLog(dt time.Duration) string {
	ticker := time.NewTicker(dt)
	defer ticker.Stop()
	var stat rmq.Stats
	var err error
	for {
		<-ticker.C
		stat, err = j.Metic()
		if err != nil {
			j.Logger.Error(err.Error())
		} else {
			j.Logger.Info("rmq metic", zap.Any("stat", stat.QueueStats))
		}
	}
}
func (j *RmqJob) Metic() (rmq.Stats, error) {
	queues, err := j.conn.GetOpenQueues()
	if err != nil {
		return rmq.Stats{}, err
	}
	return j.conn.CollectStats(queues)
}

func (j *RmqJob) Close() {
	st := time.Now()
	<-j.q.StopConsuming()
	dt := time.Since(st)
	j.Logger.Info(fmt.Sprintf(
		"%s 队列消费停止，耗时%dms",
		j.name, dt/time.Millisecond,
	))
}
