package glog

import "testing"

func setUp() {
	InitLog(&C{
		Driver:     "file",
		Path:       "D:\\Code\\gtank\\logs\\cur.log",
		MaxSize:    32,
		MaxBackups: 300,
	})
}
func TestFile(t *testing.T) {
	setUp()
	Logger.Info("aaaaaaa")
	Logger.Debug("aaaaaaa")
	Logger.Info("aaaaaaa")
	SetLevel("debug")
	Logger.Debug("aaaaaaa")
}
