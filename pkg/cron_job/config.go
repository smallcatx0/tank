package cronjob

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

const (
	CommandType_Http = "http"

	Status_Online  = "online"
	Status_Offline = "offline"
)

type CronTask struct {
	ID      int64  `gorm:"id"`
	Name    string `gorm:"name"`    // 任务名
	Desc    string `gorm:"desc"`    // 任务描述
	Cron    string `gorm:"cron"`    // cron表达式
	Ctype   string `gorm:"ctype"`   // 命令类型
	Command string `gorm:"command"` // 命令详情
	Limit   int64  `gorm:"limit"`   // 执行次数
	Status  string `gorm:"status"`  // 状态 任务状态 online:启用 offline:停用
}

// 序列化/反序列化
func (c *CronTask) String() string {
	data, _ := json.Marshal(*c)
	return string(data)
}
func (c *CronTask) Build(data string) error {
	return json.Unmarshal([]byte(data), c)
}

func (c *CronTask) Unikey() string {
	return fmt.Sprintf("%d#%s", c.ID, c.Name)
}

// TableName 表名称
func (*CronTask) TableName() string {
	return "bs_cron_task"
}

func GetCronTaskCfgs(db *gorm.DB) ([]CronTask, error) {
	cfgs := []CronTask{}
	err := db.Find(&cfgs).
		Where("status = ?", Status_Online).
		Error
	return cfgs, err
}
