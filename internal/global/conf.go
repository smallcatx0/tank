package global

import (
	"flag"
	"os"
	"sync"

	"github.com/spf13/viper"
)

// 创建实例 防止空指针访问
var AppConf = viper.New()

var (
	hostname     string
	hostnameOnce sync.Once
)

func HostName() string {
	hostnameOnce.Do(func() {
		h, err := os.Hostname()
		if err != nil {
			h = "unknow"
		}
		hostname = h
	})
	return hostname
}

// 启动参数

type FlagParam struct {
	C string
	H bool
}

var Param FlagParam

func InitFlag() {
	flag.StringVar(&Param.C, "config", "conf/app.yaml", "配置文件地址")
	flag.BoolVar(&Param.H, "help", false, "帮助")
}

func Flag() bool {
	flag.Parse()
	if Param.H {
		flag.PrintDefaults()
		return false
	}
	// 存到viper中
	AppConf.Set("flag_param.c", Param.C)
	return true
}

// 初始化配置
func MustInitAppConf(filepath string) {

	AppConf.SetConfigFile(filepath)
	AppConf.SetConfigType("yaml")
	// 设置默认值
	AppConf.SetDefault("env", "dev")
	AppConf.SetDefault("debug", true)

	err := AppConf.ReadInConfig()
	if err != nil {
		panic(err)
	}
	// TODO: 兼容 env 环境变量
}

func Env() string {
	return AppConf.GetString("env")
}

func IsDebug() bool {
	return AppConf.GetBool("debug")
}
