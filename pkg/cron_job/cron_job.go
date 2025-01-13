package cronjob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

const (
	lockKeyPre = "bs:cron_task_" // redis key 前缀
)

var (
	logFlag    = zap.String("module", "cronjob")
	logJobName = func(name string) zap.Field {
		return zap.String("job_name", name)
	}
)

type CronJob struct {
	hostname string
	cron     *cron.Cron
	Logger   *zap.Logger
	redisCli *redis.Client
	taskMapp map[string]cron.EntryID
	Funcs    map[string]func() // func 任务列表
}

func NewCronJob(logger *zap.Logger, redisCli *redis.Client) (*CronJob, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	logger.With(logFlag, zap.String("hostname", hostname))
	job := &CronJob{
		hostname: hostname,
		cron:     cron.New(),
		Logger:   logger,
		redisCli: redisCli,
		taskMapp: make(map[string]cron.EntryID),
	}
	cronLogger := &cronLog{Logger: logger, InfoLog: true}
	c := cron.New(
		cron.WithChain(
			cron.Recover(cronLogger),
			cron.SkipIfStillRunning(cronLogger),
		),
		cron.WithSeconds(), // 开启秒级定时任务
	)
	job.cron = c
	return job, nil
}

func (c *CronJob) lock(name string) bool {
	key := lockKeyPre + name
	res := c.redisCli.SetNX(
		context.Background(),
		key, c.hostname,
		0)
	if res.Err() != nil {
		c.Logger.Error("[cronjob]获取redis锁失败, key="+key, zap.Error(res.Err()))
		return false
	}
	if !res.Val() {
		c.Logger.Info("[cronjob]获取redis锁失败, 任务在其他机器上执行, key=" + key)
		return false
	}
	c.Logger.Info("[cronjob]获取redis锁成功, key=" + key)
	return true
}

func (c *CronJob) unlock(name string) bool {
	luaScript := `
local val = redis.call('get', KEYS[1])
if val == ARGV[1] then
	return redis.call('del', KEYS[1])
else
	return 0
end
	`
	key := lockKeyPre + name
	res := c.redisCli.Eval(
		context.Background(),
		luaScript,
		[]string{key},
		c.hostname)
	if res.Err() != nil {
		c.Logger.Error("[cronjob]获取redis锁失败, key="+key, zap.Error(res.Err()))
		return false
	}
	if res.Val().(int64) == 0 {
		c.Logger.Info("[cronjob]释放redis锁失败, 任务在其他机器上执行, key=" + key)
		return false
	}
	c.Logger.Info("[cronjob]释放redis锁成功" + "key:" + key)
	return true
}

func (c *CronJob) RmHostJob() {
	luaScript := `
local keys = redis.call('KEYS', KEYS[1])
local affect = 0
for _, key in ipairs(keys) do
	local val = redis.call("GET", key)
	if val == ARGV[1] then
		redis.call("DEL", key)
		affect = affect + 1
	end
end
return affect
`
	res := c.redisCli.Eval(
		context.Background(),
		luaScript,
		[]string{lockKeyPre + "*"},
		c.hostname,
	)
	if res.Err() != nil {
		c.Logger.Error("[cronjob]清除本机残余redis key 失败", zap.Error(res.Err()))
	}
	c.Logger.Info(fmt.Sprintf("[cronjob]清除本机残余redis key(%d)成功", res.Val().(int64)))
}

func (c *CronJob) SetFunc(name, spec string, f func()) error {
	// 竞争
	if !c.lock(name) {
		return nil
	}
	// 本机更新任务
	if id, ok := c.taskMapp[name]; ok {
		c.cron.Remove(id)
		delete(c.taskMapp, name)
	}
	id, err := c.cron.AddFunc(spec, f)
	if err != nil {
		c.Logger.Error("[cronjob]添加任务失败", zap.Error(err))
		return err
	}
	c.taskMapp[name] = id
	c.Logger.Info("[cronjob]添加任务成功" + fmt.Sprintf("%s(%s)", name, spec))
	return nil
}

func (c *CronJob) Remove(name string) {

	if id, ok := c.taskMapp[name]; ok {
		c.cron.Remove(id)
		delete(c.taskMapp, name)
	}
	// 清理redis锁
	c.unlock(name)
}

func (c *CronJob) Start() {
	c.cron.Start()
}

func (c *CronJob) Close() {
	for key, id := range c.taskMapp {
		c.cron.Remove(id)
		c.Remove(key)
	}
	c.cron.Stop()
}

func (c *CronJob) InitByDb(cfgs []CronTask) {
	for _, cfg := range cfgs {
		switch cfg.Ctype {
		case CommandType_Http:
			logger := c.Logger.With(logJobName(cfg.Name))
			httpCfg := &CommandHttp{}
			err := json.Unmarshal([]byte(cfg.Command), httpCfg)
			if err != nil {
				logger.Error("[cronjob]解析http任务失败", zap.Error(err))
				continue
			}
			cmd, err := MakeCommandHttp(httpCfg, logger)
			if err != nil {
				logger.Error("[cronjob]构建http任务失败", zap.Error(err))
				continue
			}
			uniJobName := fmt.Sprintf("%d#%s", cfg.ID, cfg.Name)
			err = c.SetFunc(uniJobName, cfg.Cron, cmd)
			if err != nil {
				logger.Error("[cronjob]添加任务失败", zap.Error(err))
				continue
			}
			logger.Info("[cronjob]添加任务成功")
		}
		// TODO: 支持其他类型的func

	}

}

type cronLog struct {
	Logger  *zap.Logger
	InfoLog bool
}

func (z *cronLog) Info(msg string, keysAndValues ...interface{}) {
	if z.InfoLog {
		z.Logger.Info(msg, zap.String("extral", kv2Str(keysAndValues)))
	}
}

func (z *cronLog) Error(err error, msg string, keysAndValues ...interface{}) {
	z.Logger.Error(msg, zap.Error(err), zap.String("extral", kv2Str(keysAndValues)))
}

func kv2Str(keysAndValues []interface{}) string {
	var sb strings.Builder
	for i := 0; i < len(keysAndValues); i += 2 {
		sb.WriteString(fmt.Sprint(keysAndValues[i]) + "=" + fmt.Sprint(keysAndValues[i+1]) + "; ")
	}
	return sb.String()
}

var (
	// 秒级 cron 表达式解析器
	SecCronParser = cron.NewParser(
		cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor,
	)
)

// 解析 cron 表达式 获取下次执行时间
func GetSpecNext(spec string, now time.Time) (time.Time, error) {
	sched, err := SecCronParser.Parse(spec)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(time.Now()), nil
}
