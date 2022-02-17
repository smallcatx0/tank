package rdb

import (
	"gtank/models/dao"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

func RedisInit() {
	dao.ConnRedis(&redis.Options{
		Addr: "redis.serv:6379",
		DB:   0,
	})
	dao.CachePrefix = "unitest"
}

func TestK(t *testing.T) {
	assert.Equal(t, "a:b:c:d", K("a", "b", "c", "d"))
	assert.Equal(t, "b:c:d", K("", "b", "", "c", "d"))
	assert.Equal(t, "a:c:d", K("a", "", "", "c", "d"))
}

func TestRemember(t *testing.T) {
	RedisInit()
	act, err := Remember("TestRem", func() string {
		return "hello"
	})
	assert.NoError(t, err)
	assert.Equal(t, act, "hello")

	act, err = Remember("TestRem", func() string {
		return "no hello"
	})
	assert.NoError(t, err)
	assert.Equal(t, act, "hello")
}
