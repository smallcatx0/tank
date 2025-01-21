package cronjob

import (
	"gtank/models/dao"
	"log"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

var redisCli *redis.Client

func InitRedis() {
	var err error
	redisCli, err = dao.ConnRedis(&redis.Options{
		Addr: "127.0.0.1:6379",
		DB:   0,
	})
	if err != nil {
		log.Panic(err.Error())
	}
}

var httpcfg = CommandHttp{
	Retry:  3,
	Method: "POST",
	Url:    "http://postman-echo.com/post",
	Headers: map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   "cronjob",
	},
	Body: `{"key":"value"}`,
}

func Test_CronJob(t *testing.T) {
	InitRedis()

	logger := zap.NewExample()
	job, err := NewCronJob("testcj", logger, redisCli)
	assert.NoError(t, err)
	job.Start()
	job.SetFunc("test_smpl", "*/10 * * * * *", func() {
		logger.Info("udf func")
	})

	httpCall, err := MakeCommandHttp(&httpcfg, logger)
	assert.NoError(t, err)
	job.SetFunc("test_http", "*/5 * * * * *", httpCall)
	time.Sleep(time.Minute)
	job.Close()
}

func Test_redis(t *testing.T) {
	InitRedis()

	logger := zap.NewExample()
	job, err := NewCronJob("testcj", logger, redisCli)
	assert.NoError(t, err)
	job.lock("test_1")
	job.lock("test_2")
}

func Test_RmHostJob(t *testing.T) {
	InitRedis()

	logger := zap.NewExample()
	job, err := NewCronJob("testcj", logger, redisCli)
	assert.NoError(t, err)
	job.RmHostJob()
}

var cfg = CronTask{
	ID:      3,
	Name:    "testcj",
	Desc:    "testcj",
	Cron:    "@every 5s",
	Ctype:   "shell",
	Command: "echo \"testcj\"",
	Limit:   1,
	Status:  "online",
}

func Test_MsgNotify(t *testing.T) {
	InitRedis()
	logger := zap.NewExample()
	job, err := NewCronJob("testcj", logger, redisCli)
	assert.NoError(t, err)
	job.Push(cfg)
	job.Push(cfg)
	job.Push(cfg)
	job.Push(cfg)
	job.Push(cfg)

}

func Test_ACMsg(t *testing.T) {
	InitRedis()
	logger := zap.NewExample()
	job, err := NewCronJob("testcj", logger, redisCli)
	assert.NoError(t, err)
	job.Start()
	time.Sleep(time.Second * 60)
}

func Test_cfgStrbuild(t *testing.T) {
	cfg := &CronTask{
		ID:      1,
		Name:    "testcj",
		Desc:    "testcj",
		Cron:    "@every 5s",
		Ctype:   "shell",
		Command: "1",
		Limit:   1,
		Status:  "online",
	}
	data := cfg.String()
	t.Log(string(data))
	cfg2 := &CronTask{}
	err := cfg2.Build(data)
	assert.NoError(t, err)
	assert.Equal(t, cfg, cfg2)
}
