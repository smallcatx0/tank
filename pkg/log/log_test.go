package glog_test

import (
	"testing"

	glog "gitee.com/smallcatx0/gtank/pkg/log"
)

func setUp() {
}
func TestFile(t *testing.T) {
	setUp()
	glog.InitLog2file("/home/logs/tank/curr.log", "Info")
	param := map[string]interface{}{
		"name": "kui",
		"age":  18,
	}

	glog.Debug("该条日志不应会被记录")
	glog.SetAtomLevel("debug")

	glog.Debug("test debug")
	glog.Debug("test debug with requestId", "request-12dfawedfse")
	glog.Debug("test debug with mor", "request-12dfawedfse", "extra one", "extra two")
	glog.DebugF("requestid", "测试模板日志age=%d", 23)
	glog.DebugT("requestid", "DebugT", param, param)

	glog.Info("测试INFO 级别完整信息", "request-123123123", "扩展信息1", "扩展信息2")
	glog.InfoF("", "测试模板日志name=%s", "kui")
	glog.InfoT("", "测试模板日志Json扩展信息", param, param)

	glog.Warn("测试warn级别完整信息", "request-123123123", "扩展信息1", "扩展信息2")
	glog.WarnF("", "测试模板日志name=%s", "kui")
	glog.WarnT("", "测试模板日志Json扩展信息", param, param)

	glog.Error("测试warn级别完整信息", "request-123123123", "扩展信息1", "扩展信息2")
	glog.ErrorF("", "测试模板日志name=%s", "kui")
	glog.ErrorT("", "测试模板日志Json扩展信息", param, param)

	glog.DPanic("测试DPanic级别完整信息", "request-dadfmwesd", "扩展信息1", "扩展信息2")

	glog.Sync()
}

func TestCons(t *testing.T) {
	glog.InitLog2std("info")
	glog.Debug("不会被打出来的日志")
	glog.SetAtomLevel("debug")
	glog.Debug("会被打出来的 debug日志", "rq-1231dsf", "extra1", "extra2")
}
