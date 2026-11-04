package bootstrap

import (
	"time"

	"serv_tpl/pkg/logger"
)

// Heartbeat 按开关启动进程级状态采样，不提供关闭句柄，也不影响服务退出码。
// 采样错误由 logger 记录；panic 在自身 goroutine 中恢复并结束心跳。
// 心跳随进程退出结束，允许退出期间丢失日志或产生日志写入错误。
func Heartbeat(enabled bool, interval time.Duration) {
	if !enabled {
		return
	}
	if interval <= 0 {
		logger.L().Warn("心跳间隔必须为正，已跳过")
		return
	}
	go func() {
		defer func() {
			if recover() != nil {
				logPanic("heartbeat")
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			logger.SysStatInfo(logger.L())
		}
	}()
}
