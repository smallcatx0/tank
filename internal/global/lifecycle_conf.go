package global

import (
	"fmt"
	"github.com/spf13/viper"
	"net"
	"strconv"
	"time"
)

// LifecycleConfig 是校验后的启动与退出配置快照。
type LifecycleConfig struct {
	Port                                                    int
	Debug                                                   bool
	Startup, Shutdown, ReadHeader, Read, Write, Idle, Drain time.Duration
	GRPC                                                    GRPCConfig
}

// GRPCConfig 定义可选 gRPC 监听和排空期限，仅 API 使用。
type GRPCConfig struct {
	Enabled                  bool
	Address                  string
	Drain, ConnectionTimeout time.Duration
}

// MustLoadLifecycleConfig 校验 API 配置，配置错误直接终止启动。
func MustLoadLifecycleConfig(c *viper.Viper) LifecycleConfig {
	cfg, err := LoadLifecycleConfig(c)
	if err != nil {
		panic(err)
	}
	return cfg
}

// MustLoadWorkerLifecycleConfig 只校验 worker 配置，错误直接终止启动。
func MustLoadWorkerLifecycleConfig(c *viper.Viper) LifecycleConfig {
	cfg, err := LoadWorkerLifecycleConfig(c)
	if err != nil {
		panic(err)
	}
	return cfg
}

// LoadLifecycleConfig 校验 HTTP、可选 gRPC 及共享生命周期配置。
func LoadLifecycleConfig(c *viper.Viper) (LifecycleConfig, error) {
	return loadLifecycleConfig(c, true)
}

// LoadWorkerLifecycleConfig 只校验 worker 使用的配置，不读取或校验 http.*、grpc.*。
func LoadWorkerLifecycleConfig(c *viper.Viper) (LifecycleConfig, error) {
	return loadLifecycleConfig(c, false)
}

func loadLifecycleConfig(c *viper.Viper, withHTTP bool) (LifecycleConfig, error) {
	defaults := map[string]any{"startup_timeout": "15s", "shutdown_timeout": 30}
	if withHTTP {
		for k, v := range map[string]any{"http.port": 8080, "http.debug": false,
			"grpc.enable": false, "grpc.address": "127.0.0.1:9090", "grpc.drain_timeout": "5s", "grpc.connection_timeout": "5s",
			"http.read_header_timeout": "5s", "http.read_timeout": "15s", "http.write_timeout": "15s",
			"http.idle_timeout": "60s", "http.drain_timeout": "20s"} {
			defaults[k] = v
		}
	}
	for k, v := range defaults {
		c.SetDefault(k, v)
	}
	var o LifecycleConfig
	positiveInt := func(key string) (int, error) {
		v, e := strconv.Atoi(fmt.Sprint(c.Get(key)))
		if e != nil || v <= 0 {
			return 0, fmt.Errorf("配置 %s 必须为正整数", key)
		}
		return v, nil
	}
	var err error
	if withHTTP {
		o.GRPC.Enabled, err = strconv.ParseBool(fmt.Sprint(c.Get("grpc.enable")))
		if err != nil {
			return o, fmt.Errorf("grpc.enable 必须为布尔值")
		}
		if o.GRPC.Enabled {
			o.GRPC.Address = c.GetString("grpc.address")
			_, port, e := net.SplitHostPort(o.GRPC.Address)
			n, parseErr := strconv.Atoi(port)
			if e != nil || parseErr != nil || n < 1 || n > 65535 {
				return o, fmt.Errorf("grpc.address 必须为 host:port，端口范围 1..65535")
			}
		}
	}
	if withHTTP {
		o.Port, err = positiveInt("http.port")
		if err != nil {
			return o, err
		}
		if o.Port > 65535 {
			return o, fmt.Errorf("http.port 超出范围")
		}
	}
	seconds, err := positiveInt("shutdown_timeout")
	if err != nil {
		return o, err
	}
	if int64(seconds) > int64((1<<63-1)/time.Second) {
		return o, fmt.Errorf("shutdown_timeout 超出范围")
	}
	o.Shutdown = time.Duration(seconds) * time.Second
	durations := map[string]*time.Duration{"startup_timeout": &o.Startup}
	if o.GRPC.Enabled {
		durations["grpc.drain_timeout"] = &o.GRPC.Drain
		durations["grpc.connection_timeout"] = &o.GRPC.ConnectionTimeout
	}
	if withHTTP {
		for k, p := range map[string]*time.Duration{"http.read_header_timeout": &o.ReadHeader, "http.read_timeout": &o.Read, "http.write_timeout": &o.Write, "http.idle_timeout": &o.Idle, "http.drain_timeout": &o.Drain} {
			durations[k] = p
		}
	}
	for k, p := range durations {
		d, e := time.ParseDuration(fmt.Sprint(c.Get(k)))
		if e != nil || d <= 0 {
			return o, fmt.Errorf("配置 %s 必须为正时长", k)
		}
		*p = d
	}
	if withHTTP {
		if o.Drain >= o.Shutdown {
			return o, fmt.Errorf("http.drain_timeout 必须小于 shutdown_timeout")
		}
		if o.GRPC.Enabled && o.GRPC.Drain >= o.Shutdown-o.Drain {
			return o, fmt.Errorf("HTTP 与 gRPC 排空期限之和必须小于 shutdown_timeout")
		}
		o.Debug, err = strconv.ParseBool(fmt.Sprint(c.Get("http.debug")))
		if err != nil {
			return o, fmt.Errorf("http.debug 必须为布尔值")
		}
	}
	if c.GetString("log.type") == "file" && c.GetString("log.path") == "" {
		return o, fmt.Errorf("log.path 未配置")
	}
	return o, nil
}
