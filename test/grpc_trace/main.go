// Command grpc_version 查询 API 版本，支持参数及环境变量配置地址。
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/spf13/viper"
	"os"
	"serv_tpl/internal/global"
	"serv_tpl/internal/telemetry"
	"time"

	versionv1 "gitee.com/chengduzhihuige/go-serv-template/sdk/version/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	// 步骤1：读取地址，优先级为命令行参数 > 环境变量 > 默认地址。
	defaultAddr := os.Getenv("GRPC_ADDR")
	if defaultAddr == "" {
		defaultAddr = "127.0.0.1:9090"
	}
	addr := flag.String("addr", defaultAddr, "gRPC 服务端 host:port")
	timeout := flag.Duration("timeout", 3*time.Second, "RPC 总期限")
	endpoint := flag.String("trace-endpoint", "", "可选 OTLP/HTTP 完整 URL，联调采样率为 1")
	flag.Parse()
	c := viper.New()
	c.Set("observability.service_name", "grpc-version-client")
	c.Set("observability.tracing.enable", *endpoint != "")
	c.Set("observability.tracing.endpoint", *endpoint)
	c.Set("observability.tracing.sample_ratio", 1)
	options, err := global.LoadObservabilityConfig(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	observation, err := telemetry.New(context.Background(), options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = callVersion(context.Background(), *addr, *timeout, observation.GRPCClientOption())
	closeErr := observation.Shutdown(context.Background())
	if err == nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func callVersion(parent context.Context, addr string, timeout time.Duration, options ...grpc.DialOption) error {
	if addr == "" || timeout <= 0 {
		return fmt.Errorf("addr 不能为空，timeout 必须大于 0")
	}
	// 步骤2：创建与当前明文服务端匹配的连接，函数退出时释放。
	conn, err := grpc.NewClient(addr, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, options...)...)
	if err != nil {
		return fmt.Errorf("创建 gRPC 客户端: %w", err)
	}
	defer conn.Close()
	// 步骤3：连接等待和实际调用共用一个明确的总期限。
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	client := versionv1.NewVersionServiceClient(conn)
	reply, err := client.Version(ctx, &versionv1.VersionRequest{})
	if err != nil {
		return fmt.Errorf("调用 Version: %w", err)
	}
	data, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(reply)
	if err != nil {
		return fmt.Errorf("编码响应: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
