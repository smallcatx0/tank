package dbstrategy

import (
	"gtank/models/dao"
	"log"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var test_db_dsn string
var ttl = TabledataTtl{
	ID:         1,
	UnKey:      "bs_sth_unitest",
	DbName:     "bs",
	Tablename:  "bs_sth_task",
	ColumnName: "updated_at",
	ColumnType: ColumType_Timestamp,
	Limit:      100,
	TtlValue:   18000, // 超时时间 s
}

func Test_deteleTableRecord(t *testing.T) {

	ttl.Dsn = test_db_dsn
	stg, err := NewDbStrategy(nil, nil)
	assert.NoError(t, err)
	stg.Debug = true
	err = stg.deteleTableRecord(ttl)
	assert.NoError(t, err)
}

var dbRetry = TabledataRetry{
	ID:         1,
	Unkey:      "bs_sth_job_retry",
	Dsn:        "",
	DbName:     "bs",
	Tablename:  "bs_sth_task",
	ColumnName: "updated_at",
	ColumnType: "timestamp",
	FindWh:     "`status`=30",
	SetFields:  "`status`=1",
	Before:     380000,
	Duration:   3600,
	Limit:      200,
	Spec:       "",
	Desc:       "数据库重试任务-单元测试",
}

func Test_updateTableTecord(t *testing.T) {
	dbRetry.Dsn = test_db_dsn
	stg, err := NewDbStrategy(nil, nil)
	assert.NoError(t, err)
	stg.Debug = true
	err = stg.updateTableTecord(dbRetry)
	assert.NoError(t, err)
}

var (
	redisCli *redis.Client
	dbCli    *gorm.DB
)

func MustInitDao() {
	var err error
	redisCli, err = dao.ConnRedis(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	if err != nil {
		panic(err)
	}
	dbCli, err = connDb(test_db_dsn, zap.NewExample())
	if err != nil {
		panic(err)
	}
}
func Test_cronFun(t *testing.T) {
	var (
		spec = "*/10 * * * * ?"
		fn   = func() {
			log.Println("单元测试 Func 执行一次")
		}
	)
	MustInitDao()
	stg, err := NewDbStrategy(dbCli, redisCli)
	assert.NoError(t, err)

	stg.AddCronFunc(
		spec, "unit-test-fn-01", fn,
	)
	stg.cron.Start()

	time.Sleep(time.Second * 50)
}
