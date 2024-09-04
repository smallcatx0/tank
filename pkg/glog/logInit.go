package glog

func InitLog2std(level string) {
	logger, err := NewStdLogger(level)
	if err != nil {
		panic("[init]stdout日志初始化失败")
	}
	defIns = logger
}

func InitLog2file(filename, level string) {
	// 按天切割日志
	logger, err := NewFileLogger(filename, level)
	if err != nil {
		panic("[init]stdout日志初始化失败")
	}
	defIns = logger
}
