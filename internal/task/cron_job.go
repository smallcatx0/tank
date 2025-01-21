package task

import (
	"gtank/models/dao"
	cronjob "gtank/pkg/cron_job"
	"gtank/pkg/glog"
)

// 根据数据库初始化定时任务
func StartCronJob() func() {
	job, err := cronjob.NewCronJob("testcj",
		glog.D().Z(), dao.RedisCli)
	if err != nil {
		glog.Error("[cronjob]初始化任务: " + err.Error())
		return nil
	}
	job.Start()

	// 从数据库中获取任务配置
	cfgs, err := cronjob.GetCronTaskCfgs(dao.MysqlCli)
	if err != nil {
		glog.Error("[cronjob]读取数据库失败: " + err.Error())
		return nil
	}
	job.RmHostJob()
	job.PushBat(cfgs)
	return job.Close
}
