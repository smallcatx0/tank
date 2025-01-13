package cronjob

import (
	"gorm.io/gorm"
)

const (
	CommandType_Http = "http"
)

type CronTask struct {
	ID      int64  `gorm:"id"`
	Name    string `gorm:"name"`    // 任务名
	Desc    string `gorm:"desc"`    // 任务描述
	Cron    string `gorm:"cron"`    // cron表达式
	Ctype   string `gorm:"ctype"`   // 命令类型
	Command string `gorm:"command"` // 命令详情
	Limit   int64  `gorm:"limit"`   // 执行次数
	Status  int8   `gorm:"status"`  // 状态 任务状态 1:启用 2:停用
}

// TableName 表名称
func (*CronTask) TableName() string {
	return "bs_cron_task"
}

func GetCronTaskCfgs(db *gorm.DB) ([]CronTask, error) {
	cfgs := []CronTask{}
	err := db.Find(cfgs).
		Where("status = ?", 1).
		Error
	return cfgs, err
}
