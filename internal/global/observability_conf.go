package global

import (
	"fmt"
	"github.com/spf13/viper"
	"math"
	"net"
	"net/url"
	"serv_tpl/internal/telemetry"
	"strconv"
	"strings"
	"time"
)

// LoadObservabilityConfig 仅校验启用项；预算由所有业务配置解析后统一校验。
func LoadObservabilityConfig(c *viper.Viper) (telemetry.Options, error) {
	for k, v := range map[string]any{"metrics.enable": false, "metrics.address": "127.0.0.1:9091", "metrics.shutdown_timeout": "2s", "tracing.enable": false, "tracing.endpoint": "http://127.0.0.1:4318/v1/traces", "tracing.sample_ratio": 0.1, "tracing.export_timeout": "3s", "tracing.shutdown_timeout": "5s"} {
		c.SetDefault("observability."+k, v)
	}
	var o telemetry.Options
	var err error
	boolean := func(k string) (bool, error) {
		v, e := strconv.ParseBool(fmt.Sprint(c.Get("observability." + k)))
		if e != nil {
			return false, fmt.Errorf("observability.%s 必须为布尔值", k)
		}
		return v, nil
	}
	duration := func(k string) (time.Duration, error) {
		v, e := time.ParseDuration(fmt.Sprint(c.Get("observability." + k)))
		if e != nil || v <= 0 {
			return 0, fmt.Errorf("observability.%s 必须为正时长", k)
		}
		return v, nil
	}
	if o.Metrics.Enable, err = boolean("metrics.enable"); err != nil {
		return o, err
	}
	if o.Tracing.Enable, err = boolean("tracing.enable"); err != nil {
		return o, err
	}
	o.ServiceName = c.GetString("observability.service_name")
	o.Version = c.GetString("ver")
	o.Environment = c.GetString("env")
	if o.Metrics.Enable {
		o.Metrics.Address = c.GetString("observability.metrics.address")
		host, port, e := net.SplitHostPort(o.Metrics.Address)
		n, pe := strconv.Atoi(port)
		if e != nil || pe != nil || n < 1 || n > 65535 || strings.ContainsAny(host, " /?#\t\r\n") {
			return o, fmt.Errorf("observability.metrics.address 必须为合法 host:port")
		}
		if o.Metrics.ShutdownTimeout, err = duration("metrics.shutdown_timeout"); err != nil {
			return o, err
		}
	}
	if o.Tracing.Enable {
		if strings.TrimSpace(o.ServiceName) == "" {
			return o, fmt.Errorf("observability.service_name 不能为空")
		}
		o.Tracing.Endpoint = c.GetString("observability.tracing.endpoint")
		u, e := url.Parse(o.Tracing.Endpoint)
		if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return o, fmt.Errorf("observability.tracing.endpoint 必须为不含凭据、查询参数和 fragment 的 HTTP(S) URL")
		}
		if port := u.Port(); port != "" {
			n, e := strconv.Atoi(port)
			if e != nil || n < 1 || n > 65535 {
				return o, fmt.Errorf("observability.tracing.endpoint 端口无效")
			}
		}
		o.Tracing.SampleRatio, err = strconv.ParseFloat(fmt.Sprint(c.Get("observability.tracing.sample_ratio")), 64)
		if err != nil || math.IsNaN(o.Tracing.SampleRatio) || math.IsInf(o.Tracing.SampleRatio, 0) || o.Tracing.SampleRatio < 0 || o.Tracing.SampleRatio > 1 {
			return o, fmt.Errorf("observability.tracing.sample_ratio 必须在 [0,1]")
		}
		if o.Tracing.ExportTimeout, err = duration("tracing.export_timeout"); err != nil {
			return o, err
		}
		if o.Tracing.ShutdownTimeout, err = duration("tracing.shutdown_timeout"); err != nil {
			return o, err
		}
	}
	return o, nil
}

// ValidateObservabilityBudget 用逐项扣减避免时长相加溢出；全部关闭沿用既有约束。
func ValidateObservabilityBudget(o telemetry.Options, total time.Duration, drains ...time.Duration) error {
	if !o.Metrics.Enable && !o.Tracing.Enable {
		return nil
	}
	stages := append([]time.Duration{5 * time.Second}, drains...)
	if o.Metrics.Enable {
		stages = append(stages, o.Metrics.ShutdownTimeout)
	}
	if o.Tracing.Enable {
		stages = append(stages, o.Tracing.ShutdownTimeout)
	}
	for _, d := range stages {
		if d < 0 || d >= total {
			return fmt.Errorf("shutdown_timeout 不足以覆盖业务排空、观测关闭和 5s 清理余量")
		}
		total -= d
	}
	return nil
}
