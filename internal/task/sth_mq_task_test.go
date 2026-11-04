package task

import (
	"fmt"
	"log"
	"serv_tpl/models/dao"
	sthjob "serv_tpl/pkg/sth_job"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

var redisCli *redis.Client

func InitRedis() {
	var err error
	redisCli, err = dao.OpenRedis(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	if err != nil {
		log.Panic(err.Error())
	}
}

const (
	MaxCount = 500000
)

func Test_AddTasks(t *testing.T) {
	t.SkipNow()
	InitRedis()
	mainQ, err := sthjob.NewRmqJob(redisCli, "sth_task_main")
	assert.NoError(t, err)
	for i := range MaxCount {
		t := SthMqTask{ID: i, Con: fmt.Sprintf("task %d", i)}
		mainQ.Queue().PublishBytes(t.Serialize())
	}
}
