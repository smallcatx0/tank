// Command grpc_version 查询 API 的版本信息，以 JSON 输出结果。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	versionv1 "serv_tpl/api/version/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	// 步骤1：读取客户端参数，拒绝没有有效期限的调用。
	addr := flag.String("addr", "127.0.0.1:9090", "gRPC 服务地址（明文测试连接）")
	timeout := flag.Duration("timeout", 3*time.Second, "RPC 总期限，如 3s")
	flag.Parse()
	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "timeout 必须大于 0")
		os.Exit(1)
	}
	// 步骤2：创建客户端并在总期限内执行 RPC，建连时间也计入期限。
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建 gRPC 客户端失败:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	resp, err := versionv1.NewVersionServiceClient(conn).Version(ctx, &versionv1.VersionRequest{})
	// 步骤3：os.Exit 不执行 defer，在判断退出码之前显式释放连接和计时器。
	cancel()
	closeErr := conn.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Version 失败:", err)
		os.Exit(1)
	}
	if closeErr != nil {
		fmt.Fprintln(os.Stderr, "关闭 gRPC 客户端失败:", closeErr)
		os.Exit(1)
	}
	data, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(resp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "版本信息编码失败:", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
