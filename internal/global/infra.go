package global

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"serv_tpl/pkg/kafka"
)

// 进程级基础设施实例：只在装配阶段（internal/bootstrap）写入一次，运行期只读。
// 需要参与事务或多实例时，请通过构造函数参数注入，不要再新增读取全局变量的路径。
var (
	MysqlCli *gorm.DB
	RedisCli *redis.Client
	// KafkaCli 由 worker 启动阶段装配，业务处理器通过参数注入。
	KafkaCli *kafka.Consumer
)
