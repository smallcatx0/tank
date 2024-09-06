package db_job

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type TaskStatus int

const (
	logPre = "[db_job] "
	// 任务状态流转
	TaskStatus_init   TaskStatus = 1
	TaskStatus_runing TaskStatus = 10
	// 失败的状态 由消费者自己写入。错误次数也由消费者自己维护
	// TaskStatus_fail   TaskStatus = 20
	TaskStatus_done TaskStatus = 30
)

const (
	// 全局默认配置
	LogKeyTpl      = "bs:job:%s:%s" // 分布式锁key
	BatNum         = 10
	NoTaskSleep    = 300
	NoLockedSleep  = 10
	WorkerNum      = 4
	JobBufferCount = 20
)

// 任务的抽象
type ITask interface {
	ID() int64
	TableName() string

	GetTasks(*gorm.DB, int) ([]ITask, error)
	UpdateStatus(*gorm.DB, []int64, TaskStatus) error
	StatusReset(*gorm.DB, time.Duration) (int64, error)

	Run(*gorm.DB) TaskStatus
}

// job 管理者
type DbJob struct {
	hostname     string
	JobName      string // 任务名称
	lockKey      string // 分布式锁key
	db           *gorm.DB
	redisCli     *redis.Client
	Logger       *zap.Logger
	taskIns      ITask      // 任务实例
	taskBuff     chan ITask // 任务列表缓冲区
	taskDoneBuff chan int64 // 已成功ids

	WorkerNum int // 消费者数量

	TaskBatDone      bool // 是否需要批量修改成功
	BatNum           int  // 一次从数据库拿多少任务
	NoTaskSleep      int  // 无任务,休眠时间（秒）
	NoLockedSleep    int  // 未获得锁,休眠时间（秒）
	NeedTimeoutReset bool // 是否需要超时重置
	TimeoutResetD    int  // 超时重置检察间隔（秒）
	TaskTimeout      int  // 任务超时时间（秒）
}

func NewDbJob(db *gorm.DB, rds *redis.Client, task ITask, jobName string) (*DbJob, error) {
	hostname, err := os.Hostname()
	if err != nil {
		zap.L().Error(logPre + "get hostname err:" + err.Error())
		hostname = "unknow"
	}
	lockkey := fmt.Sprintf(LogKeyTpl, task.TableName(), jobName)
	obj := DbJob{
		hostname: hostname,
		lockKey:  lockkey,
		taskIns:  task,
		db:       db,
		redisCli: rds,
	}
	// 如下参数 先使用默认配置，可在外面修改
	obj.taskBuff = make(chan ITask, JobBufferCount)
	obj.WorkerNum = WorkerNum
	obj.Logger = zap.L()
	obj.BatNum = BatNum
	obj.NoTaskSleep = NoTaskSleep
	obj.NoLockedSleep = NoLockedSleep
	obj.NeedTimeoutReset = false
	return &obj, nil
}

func (j *DbJob) SetBufferNum(num int) {
	j.taskBuff = make(chan ITask, num)
}
func (j *DbJob) SetDoneBuff(num int) {
	j.TaskBatDone = true
	j.taskDoneBuff = make(chan int64, num)
}

// 协程将任务跑起来
func (j *DbJob) GoTaskRun() {
	fn := func() {
		defer j.panicRecover()
		for {
			t := <-j.taskBuff
			status := t.Run(j.db)

			// 定期批量更新成功任务，（优化数据库压力）
			if j.TaskBatDone && status == TaskStatus_done {
				j.taskDoneBuff <- t.ID()
			}
		}
	}
	for i := 0; i < j.WorkerNum; i++ {
		go fn()
	}
}

// 从数据库中拿一批任务放入缓冲区
func (j *DbJob) dbComsumer() {
	if j.lock() {
		// 未获得锁,等待10s
		time.Sleep(time.Second * 10)
		return
	}
	var msg string
	// 从数据库中拿一批数据
	tasks, err := j.taskIns.GetTasks(j.db, j.BatNum)
	if err != nil {
		msg = logPre + fmt.Sprintf("从%s获取任务失败, err=%s",
			j.taskIns.TableName(),
			err.Error(),
		)
		j.Logger.Error(msg)
		j.unLock()
		return
	}
	if len(tasks) == 0 {
		msg = logPre + fmt.Sprintf("无任务，休眠%ds", j.NoTaskSleep)
		j.Logger.Info(msg)
		j.unLock()
		time.Sleep(time.Second * time.Duration(j.NoTaskSleep))
		return
	}
	ids := []int64{}
	for _, t := range tasks {
		ids = append(ids, t.ID())
	}
	// 将这批任务改为 进行中
	err = j.taskIns.UpdateStatus(j.db, ids, TaskStatus_runing)
	if err != nil {
		j.unLock()
		msg = logPre + fmt.Sprintf("更新任务状态失败。table=%s; ids=(%v); status=%d; err=%s",
			j.taskIns.TableName(),
			ids,
			TaskStatus_runing,
			err.Error(),
		)
		j.Logger.Error(msg)
		j.unLock()
		return
	}
	// 更改状态成功后 释放锁,让其他进程也可以消费任务
	j.unLock()
	msg = logPre + fmt.Sprintf("任务(%v)开始执行", ids)
	j.Logger.Info(msg)
	for _, t := range tasks {
		j.taskBuff <- t
	}
}

func (j *DbJob) GoDbComsumer() {
	fn := func() {
		defer j.panicRecover()
		for {
			j.dbComsumer()
		}
	}
	go fn()
}

func (j *DbJob) TimeOutReset(duration, timeout time.Duration) {
	defer j.panicRecover()
	ticker := time.NewTicker(duration)
	var msg string
	var err error
	var affected int64
	defer ticker.Stop()
	for {
		<-ticker.C
		affected, err = j.taskIns.StatusReset(j.db, timeout)
		if err != nil {
			msg = logPre + fmt.Sprintf("超时重置任务状态失败，table=%s; err=%s",
				j.taskIns.TableName(),
				err.Error(),
			)
			j.Logger.Error(msg)
		} else {
			msg = logPre + fmt.Sprintf("重置%d个任务为 init",
				affected,
			)
			j.Logger.Info(msg)
		}
	}
}
func (j *DbJob) setBatTaskDone(d time.Duration) {
	defer j.panicRecover()
	ticker := time.NewTicker(d)
	defer ticker.Stop()
	for {
		<-ticker.C
		ids := []int64{}
		for flag := true; flag; {
			select {
			case id := <-j.taskDoneBuff:
				ids = append(ids, id)
			default:
				flag = false
			}
		}
		// 200条一批，更新任务状态
		idsArr := arrBreakInt64(ids, 200)
		for _, abat := range idsArr {
			err := j.taskIns.UpdateStatus(j.db, abat, TaskStatus_done)
			if err != nil {
				msg := logPre + fmt.Sprintf("批量更表(%s)新状态失败 ids=(%v) status=%d, err=%s",
					j.taskIns.TableName(),
					abat, TaskStatus_done, err.Error(),
				)
				j.Logger.Error(msg)
				// 失败则写回队列
				for _, id := range abat {
					j.taskDoneBuff <- id
				}
			}
		}
	}
}

func (j *DbJob) lock() bool {
	return j.redisCli.SetNX(
		context.Background(),
		j.lockKey,
		j.hostname,
		300*time.Second,
	).Val()
}

func (j *DbJob) unLock() {
	err := j.redisCli.Del(
		context.Background(),
		j.lockKey,
	).Err()
	if err != nil {
		msg := fmt.Sprintf("释放分布式锁失败, reids_key(%s)删除失败，err=%s", j.lockKey, err.Error())
		j.Logger.Error(msg)
	}
}

func (j *DbJob) panicRecover() {
	if r := recover(); r != nil {
		msg := logPre + "goroutine run panic " + fmt.Sprint(r)
		j.Logger.Error(msg, zap.Stack("stack"))
	}
}

// 开跑
func (j *DbJob) Start() {
	j.GoTaskRun()
	j.GoDbComsumer()
	if j.NeedTimeoutReset {
		go j.TimeOutReset(
			time.Second*time.Duration(j.TimeoutResetD),
			time.Second*time.Duration(j.TaskTimeout),
		)
	}
	if j.TaskBatDone {
		// 每秒同步一次成功状态
		go j.setBatTaskDone(time.Second)
	}
}

func (j *DbJob) Close() {
	// TODO: 关闭buff,将缓冲区任务改为 init
}

func arrBreakInt64(arr []int64, limit int) [][]int64 {
	arrLen := len(arr)
	ret := make([][]int64, 0, arrLen/limit+1)
	for i := 0; i < arrLen; i += limit {
		j := i + limit
		if j > arrLen {
			j = arrLen
		}
		ret = append(ret, arr[i:j])
	}
	return ret
}
