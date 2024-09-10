package task

import (
	"fmt"
	"gtank/models/dao"
	sthjob "gtank/pkg/sth_job"
	"log"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

var redisCli *redis.Client

func InitRedis() {
	var err error
	redisCli, err = dao.ConnRedis(&redis.Options{
		Addr: "local.serv:6379",
	})
	if err != nil {
		log.Panic(err.Error())
	}
}

const (
	MaxCount = 1000000
)

func Test_AddTasks(t *testing.T) {
	InitRedis()
	mainQ, err := sthjob.NewRmqJob(redisCli, "sth_task_main")
	assert.NoError(t, err)
	for i := 0; i < MaxCount; i++ {
		t := SthMqTask{ID: i, Con: fmt.Sprintf("task %d", i)}
		mainQ.Queue().PublishBytes(t.Serialize())
	}
}
