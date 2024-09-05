package task

import (
	"gtank/models/dao"
	"gtank/pkg/db_job"
	"gtank/pkg/glog"
	"time"

	"gorm.io/gorm"
)

// 开始跑起来这个任务
func StartSthTask() {
	job, err := db_job.NewDbJob(dao.MDB, dao.Rdb,
		&BsSthTask{}, "try_sth_task",
	)
	if err != nil {
		glog.ErrorF("[db_job]启动任务失败 err=%s", "", err.Error())
		return
	}
	job.Logger = glog.D().Z()
	// TODO: 下面这些参数从配置文件中获取
	job.SetBufferNum(10)
	job.WorkerNum = 2
	job.BatNum = 10
	job.NoTaskSleep = 60        // 获取不到认为休眠60s
	job.NeedTimeoutReset = true // 开启超时检测
	job.TimeoutResetD = 60      // 每分钟检查一次 超时任务
	job.TaskTimeout = 300       // 5分钟在doing 状态被认为超时
	job.Start()
}

// 一张任务表
type BsSthTask struct {
	Id        int64     `gorm:"column:id"`
	Content   string    `gorm:"column:content"`
	Status    int       `gorm:"status"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

var _ db_job.ITask = &BsSthTask{}

func (BsSthTask) TableName() string {
	return "bs_sth_task"
}
func (t BsSthTask) ID() int64 {
	return t.Id
}

// 批量更新状态
func (t *BsSthTask) UpdateStatus(db *gorm.DB, ids []int64, status db_job.TaskStatus) error {
	err := db.Where("id IN ?", ids).
		Updates(BsSthTask{
			Status:    int(status),
			UpdatedAt: time.Now(),
		}).Error
	return err
}

func (t *BsSthTask) Run(db *gorm.DB) db_job.TaskStatus {
	// 模拟消费任务
	glog.InfoF("task(%d) succ %s", "", t.Id, t.Content)
	time.Sleep(time.Second)
	return db_job.TaskStatus_done
}

// 从数据库中获取一批任务
func (t *BsSthTask) GetTasks(db *gorm.DB, limit int) ([]db_job.ITask, error) {
	tasks := []BsSthTask{}
	err := db.Model(&BsSthTask{}).
		Limit(limit).
		Where("status=?", db_job.TaskStatus_init).
		Find(&tasks).Error
	res := make([]db_job.ITask, len(tasks))
	for i, v := range tasks {
		res[i] = &v
	}
	return res, err
}

// 任务重置
func (t *BsSthTask) StatusReset(db *gorm.DB, timeout time.Duration) (int64, error) {
	outtime := time.Now().Add(-timeout)
	res := db.Where("status=? AND updated_at < ?", db_job.TaskStatus_runing, outtime).
		Updates(BsSthTask{Status: int(db_job.TaskStatus_init), UpdatedAt: time.Now()})
	return res.RowsAffected, res.Error
}
