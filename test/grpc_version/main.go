// Command grpc_version 查询 API 版本，支持参数及环境变量配置地址。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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
	flag.Parse()
	if err := callVersion(context.Background(), *addr, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func callVersion(parent context.Context, addr string, timeout time.Duration) error {
	if addr == "" || timeout <= 0 {
		return fmt.Errorf("addr 不能为空，timeout 必须大于 0")
	}
	// 步骤2：创建与当前明文服务端匹配的连接，函数退出时释放。
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
