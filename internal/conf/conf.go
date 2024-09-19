package conf

import (
	"os"

	"github.com/spf13/viper"
)

var AppConf *viper.Viper

var hostname string

func HostName() string {
	if hostname != "" {
		return hostname
	}
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		return "unknow"
	}
	return hostname
}

func InitAppConf(filepath *string) error {
	AppConf = viper.New()
	AppConf.SetConfigFile(*filepath)
	AppConf.SetConfigType("yaml")

	// 设置默认
	AppConf.SetDefault("base.env", "dev")
	AppConf.SetDefault("base.debug", true)
	AppConf.SetDefault("base.http_port", "80")
	AppConf.Set("flag_param.c", *filepath)

	err := AppConf.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
}

func Env() string {
	return AppConf.GetString("env")
}

func IsDebug() bool {
	return AppConf.GetBool("debug")
}

func HttpPort() string {
	return AppConf.GetString("http_port")
}
