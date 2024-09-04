package task

import (
	"context"
	"fmt"
	"gtank/pkg/glog"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const (
	jobName = "[sthjob] "
	// 任务状态流转
	Task_init   = 1
	Task_runing = 10
	Task_fail   = 20
	Task_done   = 30
)

var (
	// 分布式锁key
	ConsumerLogKey      = "bs:task:sth_task"
	ConsumerBatNum      = 10
	ConsumerNoTaskSleep = 300
)

func StartConsumSthJob(db *gorm.DB, redisCli *redis.Client, worker, buff int) {
	job, err := NewSthJob(db, redisCli, worker, buff)
	if err != nil {
		glog.Error(jobName + "NewSthJob fail, " + err.Error())
		return
	}
	job.GoRun()
	job.GoDbComsumer()
	// 每5分钟检测一次,30分钟前的 进行中任务重置未开始
	go job.TimeOutReset(time.Minute, time.Minute*30)
}

type SthJob struct {
	hostname string
	db       *gorm.DB
	redisCli *redis.Client

	taskBuff  chan BsSthTask // 任务缓冲区
	workerLen int            // 消费者数量
}

func NewSthJob(db *gorm.DB, rds *redis.Client, worker, buff int) (*SthJob, error) {
	hostname, err := os.Hostname()
	if err != nil {
		glog.Error(jobName + "get hostname err:" + err.Error())
		hostname = "unknow"
	}
	if worker == 0 {
		worker = 2
	}
	if buff == 0 {
		buff = 10
	}
	obj := SthJob{
		hostname:  hostname,
		db:        db,
		redisCli:  rds,
		taskBuff:  make(chan BsSthTask, buff),
		workerLen: worker,
	}
	return &obj, nil
}

func (j *SthJob) Run(task BsSthTask) {
	time.Sleep(time.Second * 5)
	glog.InfoF(jobName+"task(%d) succ %s", "", task.Id, task.Content)
	task.StatusSet(j.db, Task_done)
}

func (j *SthJob) GoRun() {
	fn := func() {
		defer func() {
			if r := recover(); r != nil {
				glog.Error(jobName + "goroutine run panic " + fmt.Sprint(r))
			}
		}()
		for {
			t := <-j.taskBuff
			j.Run(t)
		}
	}
	for i := 0; i < j.workerLen; i++ {
		go fn()
	}
}

// 消费数据库中的任务
func (j *SthJob) DbComsumer() {
	if j.Lock() {
		// 未获得锁,等待10s
		time.Sleep(time.Second * 10)
		return
	}
	tasks := []BsSthTask{}
	err := j.db.Model(&BsSthTask{}).
		Limit(ConsumerBatNum).
		Where("status=?", Task_init).
		Find(&tasks).Error
	if err != nil {
		j.UnLock()
		glog.Error(jobName + "select tasktable err, " + err.Error())
		return
	}
	if len(tasks) == 0 {
		j.UnLock()
		glog.InfoF(jobName+"no task sleet %ds", "", ConsumerNoTaskSleep)
		time.Sleep(time.Second * time.Duration(ConsumerNoTaskSleep))
		return
	}
	ids := []int{}
	for _, t := range tasks {
		ids = append(ids, t.Id)
	}
	// 将这批任务改为 进行中
	err = j.db.Where("id IN ?", ids).
		Updates(
			BsSthTask{Status: Task_runing, UpdatedAt: time.Now()},
		).Error
	if err != nil {
		j.UnLock()
		glog.ErrorF(jobName+"update tasktable.ids=(%v) status=%d err, %s", "",
			ids, Task_runing, err.Error(),
		)
		return
	}
	// 更改状态成功后 释放锁,让其他进程也可以消费任务
	j.UnLock()
	glog.InfoF(jobName+"tasks(%v) start consumer", "", ids)
	for _, t := range tasks {
		j.taskBuff <- t
	}
}

func (j *SthJob) GoDbComsumer() {
	fn := func() {
		defer func() {
			if r := recover(); r != nil {
				glog.Error(jobName + "goroutine run panic " + fmt.Sprint(r))
			}
		}()
		for {
			j.DbComsumer()
		}
	}
	go fn()
}

func (j *SthJob) TimeOutReset(duration, timeout time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error(jobName + "goroutine run panic " + fmt.Sprint(r))
		}
	}()
	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	for {
		<-ticker.C
		outtime := time.Now().Add(-timeout)
		res := j.db.Where("status=? AND updated_at < ?", Task_runing, outtime).
			Updates(BsSthTask{Status: Task_init, UpdatedAt: time.Now()})
		if res.Error != nil {
			glog.Error(jobName + "update tasktable fail err:" + res.Error.Error())
		} else {
			glog.InfoF(jobName+"reset %d task to init", "", res.RowsAffected)
		}
	}
}

func (j *SthJob) Lock() bool {
	return j.redisCli.SetNX(
		context.Background(),
		ConsumerLogKey,
		j.hostname,
		300*time.Second,
	).Val()
}

func (j *SthJob) UnLock() {
	err := j.redisCli.Del(
		context.Background(),
		ConsumerLogKey,
	).Err()
	if err != nil {
		glog.ErrorF("[job]redis key(%s) del faill, %s", "", ConsumerLogKey, err.Error())
	}
}

type BsSthTask struct {
	Id        int       `gorm:"column:id"`
	Content   string    `gorm:"column:content"`
	Status    int       `gorm:"status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (BsSthTask) TableName() string {
	return "bs_sth_task"
}

func (t *BsSthTask) StatusSet(db *gorm.DB, status int) error {
	err := db.Model(t).
		Updates(BsSthTask{
			Status:    status,
			UpdatedAt: time.Now(),
		}).Error
	return err
}
