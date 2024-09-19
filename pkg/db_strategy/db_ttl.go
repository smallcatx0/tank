package dbstrategy

import (
	"context"
	"fmt"
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

type DbStrategy struct {
	Logger   *zap.Logger
	cron     *cron.Cron
	db       *gorm.DB // 配置表所在的数据库链接
	redisCli *redis.Client
	ttl      *TabledataTtl
	retry    *TabledataRetry
	cronFunc map[string]cron.EntryID
}

func NewDbStrategy() (*DbStrategy, error) {
	s := &DbStrategy{
		Logger:   zap.NewExample(),
		cronFunc: make(map[string]cron.EntryID),
		ttl:      new(TabledataTtl),
		retry:    new(TabledataRetry),
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

// TODO: 考虑如何更新定时任务策略

func (s *DbStrategy) Regist() {

	// 查询配置 cron定时任务中
	ttls, err := s.ttl.GetCfgs(s.db)
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"查询数据%s库配置失败, err=%s",
			s.ttl.TableName(), err.Error(),
		))
	} else {
		fmt.Println(ttls)
		// TODO: 依次加入定时任务中
		// 记得使用临时数据库连接，并及时释放
	}
	retrys, err := s.retry.GetCfgs(s.db)
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"查询数据%s库配置失败, err=%s",
			s.retry.TableName(), err.Error(),
		))
	} else {
		fmt.Println(retrys)
		// TODO: 依次加入定时任务中
		// 记得使用临时数据库连接，并及时释放
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

// TODO: 数据库重试逻辑
