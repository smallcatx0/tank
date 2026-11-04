package models

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// 任务状态
const (
	StatusOff     = 0 // 停用
	StatusOn      = 1 // 启用
	StatusExpired = 2 // 已过期(一次性任务执行完成后)
)

// 调度类型
const (
	SchedOnce = "once" // 一次性任务 At
	SchedCron = "cron" // 周期任务
)

// 任务类型
const (
	TypeHTTP   = "http"
	TypeShell  = "shell"
	TypeGoFunc = "gofunc"
)

// JobLog 状态
const (
	LogRunning = "running"
	LogSuccess = "success"
	LogFailed  = "failed"
)

// 触发类型
const (
	TriggerCron   = "cron"
	TriggerOnce   = "once"
	TriggerManual = "manual"
)

// 告警渠道类型(当前仅钉钉, 预留后续扩展)
const (
	AlarmTypeDing = "ding_alarm"
)

// alarmMask 脱敏占位符
const alarmMask = "******"

type Job struct {
	ID           int64      `gorm:"primaryKey; column:id" json:"id"`
	Name         string     `gorm:"size:128;not null; column:name" json:"name"`
	Description  string     `gorm:"size:255; column:description" json:"description"`             // 任务描述
	Type         string     `gorm:"size:32;not null; column:type" json:"type"`                   // http / shell / gofunc
	Status       int        `gorm:"default:0; column:status" json:"status"`                      // 0=停用, 1=启用, 2=已过期
	ScheduleType string     `gorm:"size:32;not null; column:schedule_type" json:"schedule_type"` // once / cron
	CronExpr     string     `gorm:"size:128; column:cron_expr" json:"cron_expr"`
	ExecuteAt    *time.Time `gorm:"column:execute_at" json:"execute_at"`
	Payload      string     `gorm:"type:text; column:payload" json:"payload"` // JSON 配置
	TimeoutSec   int        `gorm:"default:300; column:timeout_sec" json:"timeout_sec"`
	Alarm        string     `gorm:"size:512; column:alarm" json:"alarm"` // 告警配置 JSON: 空=不告警; {"type":"ding_alarm","name":..} 预定义 / {"type":"ding_alarm","webhook":..,"secret":..} 自定义
	NextRun      *time.Time `gorm:"column:next_run" json:"next_run"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (Job) TableName() string { return "bs_job" }

// AlarmConf 告警配置(存于 Job.Alarm 的 JSON), type 为告警渠道, 当前仅 ding_alarm
type AlarmConf struct {
	Type    string `json:"type"`
	Name    string `json:"name,omitempty"`
	Webhook string `json:"webhook,omitempty"`
	Secret  string `json:"secret,omitempty"`
}

// ParseAlarm 解析 alarm JSON, 校验 type 必填, 返回结构体供上层按 type 分发
func ParseAlarm(s string) (*AlarmConf, error) {
	if s == "" {
		return nil, errors.New("alarm 为空")
	}
	var c AlarmConf
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, err
	}
	if c.Type == "" {
		return nil, errors.New("alarm type 必填")
	}
	return &c, nil
}

// IsAlarmMasked 判定自定义告警配置是否为脱敏占位(供 Update 跳过覆盖)
func IsAlarmMasked(c *AlarmConf) bool {
	return c != nil && (strings.Contains(c.Webhook, alarmMask) || strings.Contains(c.Secret, alarmMask))
}

// MaskAlarm 对告警配置脱敏: 空/预定义(name 无 webhook)原样返回; 自定义对 webhook 的 access_token 与 secret 打码; 非法 JSON 兜底返回 ******
func MaskAlarm(s string) string {
	if s == "" {
		return ""
	}
	var c AlarmConf
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return alarmMask
	}
	// 预定义机器人: 仅引用名称, 无敏感信息
	if c.Webhook == "" && c.Secret == "" {
		return s
	}
	c.Webhook = maskWebhook(c.Webhook)
	if c.Secret != "" {
		c.Secret = alarmMask
	}
	b, err := json.Marshal(c)
	if err != nil {
		return alarmMask
	}
	return string(b)
}

// maskWebhook 隐藏 access_token 取值, 无 token 参数时整体打码, 避免越界 panic
func maskWebhook(wh string) string {
	if wh == "" {
		return ""
	}
	idx := strings.Index(wh, "access_token=")
	if idx < 0 {
		return alarmMask
	}
	return wh[:idx+len("access_token=")] + alarmMask
}

type JobLog struct {
	ID          int64      `gorm:"primaryKey; column:id" json:"id"`
	JobID       int64      `gorm:"index; column:job_id" json:"job_id"`
	JobName     string     `gorm:"size:128; column:job_name" json:"job_name"`
	TriggerType string     `gorm:"size:32; column:trigger_type" json:"trigger_type"` // cron / once / manual
	Status      string     `gorm:"size:32; column:status" json:"status"`             // running / success / failed
	Output      string     `gorm:"type:text; column:output" json:"output"`
	Error       string     `gorm:"type:text; column:error" json:"error"`
	StartedAt   *time.Time `gorm:"column:started_at" json:"started_at"`
	FinishedAt  *time.Time `gorm:"column:finished_at" json:"finished_at"`
}

func (JobLog) TableName() string { return "bs_job_log" }
