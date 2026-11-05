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

	// 设置默认（与 app.yaml 顶层键、Env()/IsDebug()/HttpPort() 访问器保持一致）
	AppConf.SetDefault("env", "dev")
	AppConf.SetDefault("debug", true)
	AppConf.SetDefault("http_port", "80")
	AppConf.SetDefault("jwt.secret", "sk") // 默认仅供开发，生产环境必须在配置中修改
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

// JwtSecret jwt签名密钥，未初始化配置时回退默认值（仅供测试/开发）
func JwtSecret() string {
	if AppConf == nil {
		return "sk"
	}
	return AppConf.GetString("jwt.secret")
}
