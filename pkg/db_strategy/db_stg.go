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

	logPre = "[db_strategy] "
)

type DbStrategy struct {
	Logger   *zap.Logger
	cron     *cron.Cron
	db       *gorm.DB // 配置表所在的数据库链接
	redisCli *redis.Client
	ttl      *TabledataTtl
	retry    *TabledataRetry
	cronFunc map[string]cron.EntryID
	Debug    bool
}

func NewDbStrategy(db *gorm.DB, redisCli *redis.Client) (*DbStrategy, error) {
	s := &DbStrategy{
		Logger:   zap.NewExample(zap.AddCaller()),
		db:       db,
		redisCli: redisCli,
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

// 根据数据库配置注册任务
func (s *DbStrategy) Regist() {
	// 查询配置 cron定时任务中
	ttls, err := s.ttl.GetCfgs(s.db)
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"查询数据%s库配置失败, err=%s",
			s.ttl.TableName(), err.Error(),
		))
	} else {
		// 依次加入定时任务中
		for _, cfg := range ttls {
			s.AddCronFunc(cfg.Spec, "ttl:"+cfg.UnKey, func() {
				s.deteleTableRecord(cfg)
			})
		}
	}
	retrys, err := s.retry.GetCfgs(s.db)
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"查询数据%s库配置失败, err=%s",
			s.retry.TableName(), err.Error(),
		))
	} else {
		fmt.Println(retrys)
		for _, cfg := range retrys {
			s.AddCronFunc(cfg.Spec, "retry:"+cfg.Unkey, func() {
				s.updateTableTecord(cfg)
			})
		}
	}
	// 启动定时任务
	s.cron.Start()
}

func (s *DbStrategy) AddCronFunc(spec string, funName string, f func()) {
	s.Logger.Info(logPre + fmt.Sprintf(
		"注册任务 %s(%s)", funName, spec,
	))
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
func (s *DbStrategy) deteleTableRecord(cfg TabledataTtl) error {
	db, err := connDb(cfg.Dsn, s.Logger)
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"数据库(%s)链接失败 err=%s",
			DsnMask(cfg.Dsn), err.Error(),
		))
		return err
	}
	if s.Debug {
		db = db.Debug()
	}
	defer CloseDb(db)
	st := time.Now()
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
	default:
		return fmt.Errorf("column_type:%s 不支持，可选：%s/%s/%s",
			cfg.ColumnType, ColumType_Unix, ColumType_Timestamp, ColumType_Datetime)
	}
	deletedNum := int64(0)
	for {
		timeout, _ := context.WithTimeout(context.Background(), time.Second*sqlTimeOut)
		res := db.WithContext(timeout).Exec(sql)
		if res.Error != nil {
			s.Logger.Error(logPre + fmt.Sprintf(
				"表(%s.%s); sql=%s, err=%s",
				cfg.DbName, cfg.Tablename, sql, res.Error.Error(),
			))
			return res.Error
		}
		deletedNum += res.RowsAffected
		if res.RowsAffected == 0 {
			break
		}
		time.Sleep(time.Second)
	}
	cost := time.Since(st)
	s.Logger.Info(logPre + fmt.Sprintf(
		"表(%s.%s) sql=%s 删除%d条，耗时%dms",
		cfg.DbName, cfg.Tablename, sql, deletedNum, cost/time.Millisecond,
	))
	return nil
}

// 数据库重试逻辑

type CountRes struct {
	Count int64 `gorm:"column:c"`
}

func (s *DbStrategy) updateTableTecord(cfg TabledataRetry) error {
	db, err := connDb(cfg.Dsn, s.Logger)
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"数据库(%s)链接失败 err=%s",
			DsnMask(cfg.Dsn), err.Error(),
		))
		return err
	}
	if s.Debug {
		db = db.Debug()
	}
	defer CloseDb(db)
	curr := time.Now()
	st := curr.Add(-time.Second * time.Duration(cfg.Before))
	ed := st.Add(time.Second * time.Duration(cfg.Duration))
	findWhere := ""
	switch cfg.ColumnType {
	case ColumType_Unix:
		findWhere = fmt.Sprintf("`%s` >= %d AND `%s` < %d",
			cfg.ColumnName, st.Unix(), cfg.ColumnName, ed.Unix())
	case ColumType_Timestamp:
		findWhere = fmt.Sprintf("`%s` >= '%s' AND `%s` < '%s'",
			cfg.ColumnName, st.Format("2006-01-02 15:04:05"), cfg.ColumnName, ed.Format("2006-01-02 15:04:05"),
		)
	case ColumType_Datetime:
		findWhere = fmt.Sprintf("`%s` >= '%s' AND `%s` < '%s'",
			cfg.ColumnName, st.Format("2006-01-02 15:04:05"), cfg.ColumnName, ed.Format("2006-01-02 15:04:05"),
		)
	default:
		return fmt.Errorf("column_type:%s 不支持，可选：%s/%s/%s",
			cfg.ColumnType, ColumType_Unix, ColumType_Timestamp, ColumType_Datetime)
	}
	findWhere += " AND " + cfg.FindWh
	findSql := fmt.Sprintf(
		"SELECT count(*) c FROM `%s` WHERE %s",
		cfg.Tablename, findWhere,
	)
	updateSql := fmt.Sprintf(
		"UPDATE `%s` SET %s WHERE %s",
		cfg.Tablename, cfg.SetFields, findWhere,
	)
	s.Logger.Info("[findSql] " + findSql)
	s.Logger.Info("[updateSql] " + updateSql)
	res := CountRes{}
	err = db.Raw(findSql).First(&res).Error
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"表(%s.%s); sql=%s, err=%s",
			cfg.DbName, cfg.Tablename, findSql, err.Error(),
		))
		return err
	}
	if res.Count == 0 {
		// 不需要更新数据
		dt := time.Since(curr)
		s.Logger.Info(logPre + fmt.Sprintf(
			"表(%s.%s) sql=%s 需无需更新，耗时%dms",
			cfg.DbName, cfg.Tablename, findSql, dt/time.Millisecond,
		))
		return nil
	}
	timeout, _ := context.WithTimeout(context.Background(), 30*time.Second)
	err = db.WithContext(timeout).Exec(updateSql).Error
	if err != nil {
		s.Logger.Error(logPre + fmt.Sprintf(
			"表(%s.%s); sql=%s, err=%s",
			cfg.DbName, cfg.Tablename, findSql, err.Error(),
		))
		return err
	}
	// TODO: 需要更新量，超过某阈值 告警

	return nil
}
