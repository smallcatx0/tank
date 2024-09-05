package task

import (
	"gorm.io/gorm"
)

// 数据库任务消费者
type DbComsumer struct {
	TaskName    string //  任务名称
	LockKey     string // 分布式锁的key
	NoLockSleep int    // 未抢到分布式锁休眠时间
	TaskNum     int    // 消费者数量
	BatCount    int    // 一次从数据库拿多少任务
	NoTaskSleep int    // 无任务 休眠时间

	HostName string
	Db       *gorm.DB
}