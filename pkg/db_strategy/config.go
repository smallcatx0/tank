package dbstrategy

import (
	"crypto/md5"
	"fmt"
	"os"

	"gorm.io/gorm"
)

const (
	ColumType_Unix      = "unix"      // 时间戳数据库中 整数保存时间
	ColumType_Timestamp = "timestamp" // 时间戳
	ColumType_Datetime  = "datetime"  // 日期时间
)

var hostname string

func HostName() string {
	if hostname != "" {
		return hostname
	}
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		return "unknow"
	}
	return hostname
}

type TabledataRetry struct {
	ID        int64  `gorm:"id"`
	Unkey     string `gorm:"unkey"`      // 任务唯一名
	Dsn       string `gorm:"dsn"`        // 数据库链接
	Tablename string `gorm:"table_name"` // 表名
	Before    int64  `gorm:"before"`     // 从当前时间之前多少秒
	Limit     int64  `gorm:"limit"`      // 一次执行条数
	Findsql   string `gorm:"findsql"`    // 查询SQL
	Updatesql string `gorm:"updatesql"`  // 更新SQL
	Spec      string `gorm:"spec"`       // cron表达式
	Desc      string `gorm:"desc"`       // 描述
}

func (*TabledataRetry) TableName() string {
	return "tabledata_retry"
}

func (t *TabledataRetry) GetCfgs(db *gorm.DB) ([]TabledataRetry, error) {
	cfgs := []TabledataRetry{}
	err := db.Find(cfgs).Error
	return cfgs, err
}

type TabledataTtl struct {
	ID         int64  `gorm:"id"`
	UnKey      string `grom:"unkey"`       // 策略唯一key
	Dsn        string `gorm:"dsn"`         // 数据库链接
	DbName     string `gorm:"db_name"`     // 数据库名
	Tablename  string `gorm:"table_name"`  // 表名
	ColumnName string `gorm:"column_name"` // 依据字段名
	ColumnType string `gorm:"column_type"` // 依据字段类型
	TtlValue   int64  `gorm:"ttl_value"`   // TTL过期时间
	Limit      int64  `gorm:"limit"`       // 一次执行条数
	Spec       string `gorm:"spec"`        // cron表达式
	Desc       string `gorm:"desc"`        // 描述
}

func (*TabledataTtl) TableName() string {
	return "tabledata_ttl"
}

func (t *TabledataTtl) GetCfgs(db *gorm.DB) ([]TabledataTtl, error) {
	ttls := []TabledataTtl{}
	err := db.Find(ttls).Error
	return ttls, err
}

func (t *TabledataTtl) md5() string {
	h := md5.New()
	h.Write([]byte(t.TableName() + t.UnKey + t.Spec))
	return fmt.Sprintf("%x", h.Sum(nil))
}
