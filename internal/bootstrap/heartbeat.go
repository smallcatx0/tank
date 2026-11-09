package bootstrap

import (
	"time"

	"github.com/spf13/viper"

	"serv_tpl/pkg/logger"
)

// startHeartbeat 直接读取可选配置，采用 Viper 的宽松类型转换。
// 心跳为进程级采样，不提供关闭句柄，也不影响服务退出码。
// 采样错误由 logger 记录；panic 在自身 goroutine 中恢复并结束心跳。
// 心跳随进程退出结束，允许退出期间丢失日志或产生日志写入错误。
func startHeartbeat(c *viper.Viper) {
	c.SetDefault("metrics.enable", false)
	c.SetDefault("metrics.dt", 10)
	if !c.GetBool("metrics.enable") {
		return
	}
	seconds := c.GetInt64("metrics.dt")
	if seconds <= 0 || seconds > int64((1<<63-1)/time.Second) {
		logger.L().Warn("心跳间隔无效或超出范围，已跳过")
		return
	}
	interval := time.Duration(seconds) * time.Second
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
