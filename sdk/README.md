# Go RPC SDK

模块：`serv_tpl/sdk`，Go 1.26.0。仅包含生成的 protobuf 消息及 gRPC 客户端/服务端接口，运行依赖为 gRPC 和 protobuf，不依赖服务端根模块、数据库或业务配置。生成代码保留工具英文注释。

协议唯一来源为 [api/](../api/README.md)，服务端启动见 [业务接入指南第 6 步](../业务接入指南.md#6-开启并启动-api)。

## 生成 SDK

工具安装与固定版本见 [生成工具说明](../业务接入指南.md#1-准备生成工具)。仓库根用 PowerShell 5.1 及以上（`powershell`）或 PowerShell 7（`pwsh`）执行：

```powershell
powershell -NoProfile -File scripts/generate-sdk.ps1
go -C sdk mod tidy
go -C sdk test ./...
go -C sdk vet ./...
go -C sdk build ./...
go test ./controller/rpc ./routes ./internal/bootstrap ./test/grpc_version
go build ./cmd/...
git diff --check
git diff -- api sdk
```

Linux、macOS 或 Git Bash 可将上述第一条命令替换为：

```bash
bash scripts/generate-sdk.sh
```

Shell 入口要求 Bash（不使用 `sh`），三个生成工具须在当前环境 PATH 中；支持从其他目录通过脚本绝对路径调用。后续 `go` 和 `git` 检查命令相同，在仓库根目录执行。

每条命令成功后再继续。两个脚本使用相同版本校验和生成参数，并生成 api/ 下全部协议。单个协议的等价命令（仓库根执行）：

```powershell
protoc --proto_path=. --go_out=sdk --go_opt=module=serv_tpl/sdk --go-grpc_out=sdk --go-grpc_opt=module=serv_tpl/sdk api/version/v1/version.proto
```

`go_package` 指向 `.../sdk/version/v1;versionv1`，`module=` 去掉模块前缀，输出 `sdk/version/v1/version.pb.go` 和 `version_grpc.pb.go`，见 [Protobuf 官方生成规则](https://protobuf.dev/reference/go/go-generated/)。新增协议必须使用 SDK 模块内的 go_package。删除或移动协议时同步删除旧生成文件；脚本不会自动清理旧文件。

协议、工具和参数不变时再生成应无内容变化。协议和生成文件一起提交。根目录 `go test ./...` 不包含嵌套 SDK 模块，两个模块必须分别检查。根 go.mod 的 `require .../sdk v0.0.0` 和 `replace .../sdk => ./sdk` 用于本仓库开发及构建；SDK 自身无 replace，不得依赖根模块。无需 go.work。

## 发布 SDK

当前模块路径 `serv_tpl/sdk` 是本仓库内部占位，不经远程模块代理分发；对外提供前需拆为独立仓库并换成带域名的模块路径。拆分时按序执行：

1. 在独立仓库建立 SDK 模块，将 `sdk/` 下的生成代码与 go.mod/go.sum 迁入，`module` 改为该仓库地址（如 `example.com/<org>/<repo>/sdk`）。
2. 同步修改 `api/` 下所有协议的 `go_package`、`scripts/generate-sdk.*` 的 `module` 变量，以及根 go.mod 的 `require`/`replace`。
3. 完成生成与双模块检查后，在已审核的提交上打子模块标签（必须带 `sdk/` 目录前缀，消费者版本号仍写 `v0.1.0`）：

```powershell
git tag -a sdk/v0.1.0 -m "Release Go RPC SDK v0.1.0" <已审核的提交SHA>
git push origin refs/tags/sdk/v0.1.0
```

4. 若发布 v2，模块和 import 路径需增加 `/v2`。见 [Go 官方模块源码管理](https://go.dev/doc/modules/managing-source)。

未完成上述迁移前，其他项目只能通过本地 `replace` 引用本仓库 `sdk/`。

## 客户端安装与 import

客户端只需 Go，无需 protoc。新建独立项目，并以本地 `replace` 指向本仓库 `sdk/`（换成你的检出路径）：

```powershell
New-Item -ItemType Directory rpc-client
Set-Location rpc-client
go mod init example.com/rpc-client
go mod edit '-require=serv_tpl/sdk@v0.0.0'
go mod edit '-replace=serv_tpl/sdk=D:/code-my/tank/sdk'
```

写入下面 main.go 后执行 `go mod tidy`。SDK 拆为独立仓库并发布后，改用 `go mod edit -dropreplace=serv_tpl/sdk` 加远程 `go get`；私有仓库需配置 Git 权限并将新模块路径加入 GOPRIVATE，凭据仍需单独配置。

import 路径为 `serv_tpl/sdk/version/v1`，地址、凭据、超时和连接复用由客户端负责。
## 编写客户端

在客户端根目录新建 `main.go`，完整内容如下：

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "time"

    versionv1 "serv_tpl/sdk/version/v1"

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
```

## 配置地址并调用

仍在客户端根目录执行：

```powershell
go mod tidy
go build ./...
go run . -addr 127.0.0.1:9090 -timeout 3s
```

也可以使用此示例客户端实现的环境变量配置地址：

```powershell
$env:GRPC_ADDR = '192.168.1.20:9090'
go run . -timeout 3s
Remove-Item Env:GRPC_ADDR
```

显式传入 `-addr` 时覆盖环境变量。`GRPC_ADDR` 是上面客户端代码读取的变量，不是生成 SDK 自带的配置，也不会修改服务端配置。

| 场景 | 服务端配置 `grpc.address` | 客户端 `-addr` |
| --- | --- | --- |
| 同机调用 | `127.0.0.1:9090` | `127.0.0.1:9090` |
| 跨机器调用 | `0.0.0.0:9090` 或可达网卡 IP | 服务端可达 IP/DNS 加端口，例如 `192.168.1.20:9090` |
| 容器暴露服务 | 容器内监听 `0.0.0.0:9090` 并发布端口 | 宿主机地址与发布的端口 |

客户端填写 `host:port`，不填 HTTP 的 8080 端口、`http://`、URL 路径或监听通配地址 `0.0.0.0`。跨机器访问还需确保网络和防火墙放行；当前示例使用明文 gRPC，若服务入口启用 TLS，客户端须改用匹配的 TLS credentials 和信任证书。

成功时输出类似：

```json
{"name":"app","desc":"应用程序配置","version":"1.0.0"}
```

实际值来自服务端配置的 `name/desc/ver`。`VersionRequest` 没有字段，所以传空结构体；其他方法按生成的请求类型填写字段，调用对应 `client.Method(ctx, req)`。`grpc.NewClient` 创建通道不代表已经连通，实际 RPC 的期限覆盖连接等待及请求执行。长驻业务客户端应按目标服务复用连接，在进程退出时关闭，并从上游 context 派生调用期限。


## 本仓库快速联调

```powershell
go run ./test/grpc_version -addr 127.0.0.1:9090 -timeout 3s
```

该客户端同样支持 GRPC_ADDR，源码见 [版本查询客户端](../test/grpc_version/main.go)。

## 收到 proto 后自行生成

其他语言客户端或需自行生成的 Go 项目仍可取得 api/ 下完整协议及其 import 依赖。普通 Go 客户端优先直接 import 上述 SDK。同一进程不要同时 import 官方 SDK 和重新生成的同名协议，以免描述符重复注册。

以 Go 项目 `example.com/rpc-client` 为例：将收到的文件复制到客户端 `api/version/v1/version.proto`，安装前述生成工具，在客户端模块根执行：

```powershell
protoc --proto_path=. --go_out=. --go_opt=paths=source_relative '--go_opt=Mapi/version/v1/version.proto=example.com/rpc-client/api/version/v1;versionv1' --go-grpc_out=. --go-grpc_opt=paths=source_relative '--go-grpc_opt=Mapi/version/v1/version.proto=example.com/rpc-client/api/version/v1;versionv1' api/version/v1/version.proto
go get google.golang.org/grpc@v1.85.0 google.golang.org/protobuf@v1.36.12
```

输出为客户端 api/version/v1 下两个 .pb.go 文件。上面调用示例仅将 import 改为 `example.com/rpc-client/api/version/v1`；地址和调用步骤相同。M 参数分别覆盖两种生成器的 Go 路径，不改变协议包、字段编号或 RPC 方法名。多文件依赖须为每个需要本地映射的协议同时提供两种 M 参数；标准协议可追加 protoc/include 搜索路径。
