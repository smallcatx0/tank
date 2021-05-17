package glog_test

import (
	"testing"

	"gitee.com/smallcatx0/gtank/pkg/glog"
	"github.com/stretchr/testify/assert"
)

var webHook = "https://oapi.dingtalk.com/robot/send?access_token=90526e10d036265881023da81c1740240a4ac3434954810de42319d074b841ac"
var secret = "SECfa8c17407ea9d632eef8c09e6ad205049b95c7beb8b809f4298af306460f1d23"

func TestTextMsg(t *testing.T) {
	assert := assert.New(t)
	ala := glog.DingAlarmNew(webHook, secret)
	err := ala.Text("测试普通消息").AtPhones("18681636749").Send()
	assert.NoError(err)
}
