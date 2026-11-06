# Go RPC SDK

模块：`gitee.com/chengduzhihuige/go-serv-template/sdk`，Go 1.26.0。仅包含生成的 protobuf 消息及 gRPC 客户端/服务端接口，运行依赖为 gRPC 和 protobuf，不依赖服务端根模块、数据库或业务配置。生成代码保留工具英文注释。

协议唯一来源为 [api/](../api/README.md)，服务端启动见 [业务接入指南](../业务接入指南.md#启动-api)。

## 准备生成工具

需要三个工具：`protoc` 编译协议，`protoc-gen-go` 生成消息类型，`protoc-gen-go-grpc` 生成客户端及服务端接口。版本以本仓库生成文件头为依据，固定使用 protoc 对应的 v33.0 发布包（生成文件头记录为 v6.33.0）、protoc-gen-go v1.36.12、protoc-gen-go-grpc v1.6.0。

从 [Protobuf v33.0 官方发布页](https://github.com/protocolbuffers/protobuf/releases/tag/v33.0) 下载适合系统的 protoc 压缩包。Windows x64 使用对应 win64 包，解压后将包含 `protoc.exe` 的 `bin` 目录加入 PATH；保留配套 `include` 目录，引用标准协议时会用到。

安装两个 Go 插件，并把安装目录加入当前终端的 PATH：

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.0
grpc_tools_bin="$(go env GOBIN)"
if [ -z "$grpc_tools_bin" ]; then
  grpc_tools_bin="$(go env GOPATH)/bin"
fi
# Git Bash 将 Windows Go 返回的路径转换为 Bash 路径。
if command -v cygpath >/dev/null 2>&1; then
  grpc_tools_bin="$(cygpath -u "$grpc_tools_bin")"
fi
export PATH="$grpc_tools_bin:$PATH"
protoc --version
protoc-gen-go --version
protoc-gen-go-grpc --version
```

每条安装命令成功后再继续。PATH 调整只对当前 Bash 会话生效；长期使用可将设置写入 Bash 启动配置。工具安装方式参考 [gRPC Go 官方入门](https://grpc.io/docs/languages/go/quickstart/)，本仓库使用上述固定版本，不跟随 `latest` 自动升级。

对应版本的实际输出为 `libprotoc 33.0`、`protoc-gen-go.exe v1.36.12`（Windows）及 `protoc-gen-go-grpc 1.6.0`；生成文件头中的 `protoc v6.33.0` 与这里的 `libprotoc 33.0` 属于同一版本，不需要另找名为 6.33.0 的编译器压缩包。修改用户 PATH 后应重新打开终端；仍继承旧环境的 IDE 需重启后再打开终端。

## 生成 SDK

首次使用先完成上方[工具准备](#准备生成工具)。仓库根使用 Bash 执行：

```bash
bash scripts/generate-sdk.sh
go -C sdk mod tidy
go -C sdk test ./...
go -C sdk vet ./...
go -C sdk build ./...
go test ./controller/rpc ./routes ./internal/bootstrap ./test/grpc_version
go build ./cmd/...
git diff --check
git diff -- api sdk
```

生成脚本适用于 Linux、macOS、Git Bash 或 WSL，需使用 Bash 执行（不使用 `sh`）。三个生成工具须在当前环境 PATH 中；支持通过脚本绝对路径调用。后续 `go` 和 `git` 检查命令在仓库根执行。WSL 中需安装 Linux 版 Go 和生成工具。

每条命令成功后再继续。脚本校验工具版本并生成 api/ 下全部协议。单个协议的等价命令（仓库根执行）：

```bash
protoc --proto_path=. --go_out=sdk --go_opt=module=gitee.com/chengduzhihuige/go-serv-template/sdk --go-grpc_out=sdk --go-grpc_opt=module=gitee.com/chengduzhihuige/go-serv-template/sdk api/version/v1/version.proto
```

`go_package` 指向 `.../sdk/version/v1;versionv1`，`module=` 去掉模块前缀，输出 `sdk/version/v1/version.pb.go` 和 `version_grpc.pb.go`，见 [Protobuf 官方生成规则](https://protobuf.dev/reference/go/go-generated/)。新增协议必须使用 SDK 模块内的 go_package。删除或移动协议时同步删除旧生成文件；脚本不会自动清理旧文件。

协议、工具和参数不变时再生成应无内容变化。协议和生成文件一起提交。根目录 `go test ./...` 不包含嵌套 SDK 模块，两个模块必须分别检查。根 go.mod 的 `require .../sdk v0.0.0` 和 `replace .../sdk => ./sdk` 用于本仓库开发及构建；SDK 自身无 replace，不得依赖根模块。无需 go.work。

## 发布 SDK

当前尚未发布 SDK 版本。`v0.1.0` 是首个版本示例，发布成功前远程 go get 不可用。

1. 完成上述生成和双模块检查，将协议、SDK 源码、go.mod/go.sum 和调用方迁移提交到同一仓库，审查待发布提交，避免包含无关改动。
2. 将审核后的提交推送到远程目标分支，再在该提交上创建子模块标签（替换提交 SHA）：

```bash
release_commit='替换为已审核的提交SHA'
git tag -a sdk/v0.1.0 -m "Release Go RPC SDK v0.1.0" "$release_commit"
git push origin refs/tags/sdk/v0.1.0
```

3. 在没有 replace 的新客户端项目执行下面的 go get、构建和真实调用，核验远程获取链路。以上是操作说明，本次实现没有推送或打标签。

子模块标签必须带 `sdk/` 目录前缀，消费者版本仍写 `v0.1.0`，见 [Go 官方模块源码管理](https://go.dev/doc/modules/managing-source)。后续按语义版本发布，不移动已发布标签；若发布 v2，模块和 import 路径需增加 `/v2`。从模板创建新仓库时同步修改 SDK module、所有 go_package/import、生成脚本和根 require/replace。

## 客户端安装与 import

客户端只需 Go，无需 protoc。新建独立项目：

```bash
mkdir -p rpc-client
cd rpc-client
go mod init example.com/rpc-client
# 仅在相应标签发布后执行
go get gitee.com/chengduzhihuige/go-serv-template/sdk@v0.1.0
```

首次发布前可跳过 go get，改用本地 SDK（替换检出路径）：

```bash
go mod edit '-require=gitee.com/chengduzhihuige/go-serv-template/sdk@v0.0.0'
sdk_dir='/path/to/go-serv-template/sdk' # 替换为本机 SDK 的绝对路径
go mod edit "-replace=gitee.com/chengduzhihuige/go-serv-template/sdk=$sdk_dir"
```

写入下面 main.go 后执行 `go mod tidy`。转为发布版本时先 `go mod edit -dropreplace=gitee.com/chengduzhihuige/go-serv-template/sdk`，再 `go get gitee.com/chengduzhihuige/go-serv-template/sdk@实际版本`。私有仓库须配置 Git 权限，并将该模块加入现有 GOPRIVATE 列表（保留已有项），例如未配置时 `go env -w GOPRIVATE=gitee.com/chengduzhihuige/go-serv-template`；该设置不能代替访问凭据。

import 路径为 `gitee.com/chengduzhihuige/go-serv-template/sdk/version/v1`，地址、凭据、超时和连接复用由客户端负责。
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
```

## 配置地址并调用

仍在客户端根目录执行：

```bash
go mod tidy
go build ./...
go run . -addr 127.0.0.1:9090 -timeout 3s
```

也可以使用此示例客户端实现的环境变量配置地址：

```bash
export GRPC_ADDR='192.168.1.20:9090'
go run . -timeout 3s
unset GRPC_ADDR
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

```bash
go run ./test/grpc_version -addr 127.0.0.1:9090 -timeout 3s
```

该客户端同样支持 GRPC_ADDR，源码见 [版本查询客户端](../test/grpc_version/main.go)。

## 收到 proto 后自行生成

其他语言客户端或需自行生成的 Go 项目仍可取得 api/ 下完整协议及其 import 依赖。普通 Go 客户端优先直接 import 上述 SDK。同一进程不要同时 import 官方 SDK 和重新生成的同名协议，以免描述符重复注册。

以 Go 项目 `example.com/rpc-client` 为例：将收到的文件复制到客户端 `api/version/v1/version.proto`，安装前述生成工具，在客户端模块根执行：

```bash
protoc --proto_path=. --go_out=. --go_opt=paths=source_relative '--go_opt=Mapi/version/v1/version.proto=example.com/rpc-client/api/version/v1;versionv1' --go-grpc_out=. --go-grpc_opt=paths=source_relative '--go-grpc_opt=Mapi/version/v1/version.proto=example.com/rpc-client/api/version/v1;versionv1' api/version/v1/version.proto
go get google.golang.org/grpc@v1.85.0 google.golang.org/protobuf@v1.36.12
```

输出为客户端 api/version/v1 下两个 .pb.go 文件。上面调用示例仅将 import 改为 `example.com/rpc-client/api/version/v1`；地址和调用步骤相同。M 参数分别覆盖两种生成器的 Go 路径，不改变协议包、字段编号或 RPC 方法名。多文件依赖须为每个需要本地映射的协议同时提供两种 M 参数；标准协议可追加 protoc/include 搜索路径。
