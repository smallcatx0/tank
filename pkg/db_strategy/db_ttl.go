package dbstrategy

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	sqlTimeOut = 60 // sql执行超时
	LockKeyTpl = "bs:dbauto:%s"

	logPre = "[db_strategy]"
)

var (
	hostname = ""
)

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

// TabledataTtl 数据TTL策略表
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

func (t *TabledataTtl) GetTTLCfg(db *gorm.DB) ([]TabledataTtl, error) {
	ttls := []TabledataTtl{}
	err := db.Find(ttls).Error
	return ttls, err
}

func (t *TabledataTtl) md5() string {
	h := md5.New()
	h.Write([]byte(t.TableName() + t.UnKey + t.Spec))
	return fmt.Sprintf("%x", h.Sum(nil))
}

const (
	ColumType_Unix      = "unix"      // 时间戳数据库中 整数保存时间
	ColumType_Timestamp = "timestamp" // 时间戳
	ColumType_Datetime  = "datetime"  // 日期时间
)

type DbStrategy struct {
	Logger   *zap.Logger
	cron     *cron.Cron
	db       *gorm.DB // 配置表所在的数据库链接
	redisCli *redis.Client
	cronFunc map[string]cron.EntryID
}

func NewDbStrategy() (*DbStrategy, error) {
	s := &DbStrategy{
		Logger:   zap.NewExample(),
		cronFunc: make(map[string]cron.EntryID),
	}
	s.cron = newSecCron()
	return s, nil
}

func newSecCron() *cron.Cron {
	parser := cron.NewParser(cron.Second | cron.Minute |
		cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor,
	)
	logger := cron.DefaultLogger
	return cron.New(
		cron.WithParser(parser),
		cron.WithChain(
			cron.Recover(logger),
			cron.SkipIfStillRunning(logger),
		),
	)
}

func (s *DbStrategy) Regist() {
	// 查询配置 cron定时任务中
	ttls, err := new(TabledataTtl).GetTTLCfg(s.db)
	if err != nil {
		s.Logger.Error(logPre + "查询数据库配置失败" + err.Error())
	} else {
		fmt.Println(ttls)
		// TODO: 依次加入定时任务中
	}

}

func (s *DbStrategy) AddCronFunc(spec string, funName string, f func()) {
	s.Logger.Info(logPre + "注册任务 " + funName)
	// 重复添加任务定为更新
	if id, ok := s.cronFunc[funName]; ok {
		s.cron.Remove(id)
	}
	funcID, err := s.cron.AddFunc(spec, func() {
		// 竞争分布式锁
		if !s.lock(funName) {
			s.Logger.Info(logPre + fmt.Sprintf("%s 未抢到 %s 任务", HostName(), funName))
		} else {
			f()
			s.unlock(funName)
		}
	})
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf("%s 注册定时任务%s失败, err=%s", HostName(), funName, err.Error()))
		return
	}
	s.cronFunc[funName] = funcID
}

func (s *DbStrategy) lock(funcName string) bool {
	unkey := fmt.Sprintf(LockKeyTpl, funcName)
	return s.redisCli.SetNX(
		context.Background(),
		unkey,
		HostName(),
		300*time.Second,
	).Val()
}

func (s *DbStrategy) unlock(funcName string) {
	unkey := fmt.Sprintf(LockKeyTpl, funcName)
	err := s.redisCli.Del(
		context.Background(),
		unkey,
	).Err()
	if err != nil {
		msg := fmt.Sprintf("释放分布式锁失败, reids_key(%s)删除失败，err=%s", unkey, err.Error())
		s.Logger.Error(msg)
	}
}

// 数据库删除逻辑
func (s *DbStrategy) deteleTableRecord(db *gorm.DB, cfg TabledataTtl) error {
	dt := time.Second * time.Duration(cfg.TtlValue)
	ttl := time.Now().Add(-dt)
	var sql string
	switch cfg.ColumnType {
	case ColumType_Unix:
		sql = fmt.Sprintf("DELETE FROM %s WHERE %s < %d LIMIT %d",
			cfg.Tablename, cfg.ColumnName, ttl.Unix(), cfg.Limit)
	case ColumType_Timestamp:
		sql = fmt.Sprintf("DELETE FROM %s WHERE %s < '%s' LIMIT %d",
			cfg.Tablename, cfg.ColumnName, ttl.Format("2006-01-02 15:04:05"), cfg.Limit)
	case ColumType_Datetime:
		sql = fmt.Sprintf("DELETE FROM %s WHERE %s < '%s' LIMIT %d",
			cfg.Tablename, cfg.ColumnName, ttl.Format("2006-01-02 15:04:05"), cfg.Limit)
	}
	deletedNum := int64(0)
	for {
		timeout, _ := context.WithTimeout(context.Background(), time.Second*sqlTimeOut)
		res := db.WithContext(timeout).Exec(sql)
		if db.Error != nil {
			s.Logger.Error(logPre + "删除数据失败 err=" + db.Error.Error())
			return db.Error
		}
		deletedNum += res.RowsAffected
		if res.RowsAffected == 0 {
			break
		}
		time.Sleep(time.Second)
	}
	s.Logger.Info(fmt.Sprintf(logPre+"sql=%s 删除%d条", sql, deletedNum))
	return nil
}
