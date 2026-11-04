# gRPC 版本查询

查询方法：`version.v1.VersionService/Version`。请求为空，返回 `name`、`desc`、`version`。
与 HTTP `GET /` 一致，分别读取配置键 `name`、`desc`、`ver`；未配置时返回空字符串。
旧 demo.v1 协议已移除；客户端需按新的 protobuf 包名重新生成，调用 `/version.v1.VersionService/Version`。

## 启动和调用

以下命令在仓库根目录执行。

1. 将 `conf/app.yaml.example` 复制为 `conf/app.yaml`，填写可用的 MySQL/Redis 配置。API 仍需初始化存储，版本查询本身不使用存储。
2. 在配置中将 `grpc.enable` 设为 `true`；默认地址为 `127.0.0.1:9090`，其他期限保留模板默认值。
3. 在一个终端启动 API：

```powershell
go run ./cmd/api -config conf/app.yaml
```

在另一个终端调用：

```powershell
go run ./test/grpc_version -addr 127.0.0.1:9090 -timeout 3s
```

按配置模板启动时，预期输出（JSON 空白可能不同）：

```json
{"name":"app","desc":"应用程序配置","version":"1.0.0"}
```

客户端不再接受 name 参数，JSON 输出保留三个字段，包括空字符串。超时参数必须大于 0；服务不可达时可能立即返回 `Unavailable`，并不保证一定等待到期限。

当前客户端使用明文连接，适用于本地查询。未开启反射，不需要安装 grpcurl；公网访问需另外配置传输保护与鉴权。

## 无存储验证

```powershell
go test -count=1 ./controller/rpc
go test -count=1 ./internal/bootstrap -run '^TestGRPCVersionMatchesHTTP$'
```

第二条命令直接通过 MustGrpcServ 在随机回环端口启动服务，以生成客户端进行真实 protobuf RPC 往返，不初始化 MySQL/Redis。

## 代码位置

- [协议](../api/version/v1/version.proto)：请求、响应及方法契约。
- [实现](../controller/rpc/version.go)：读取服务版本配置。
- [装配](../internal/bootstrap/grpc.go)：在监听前直接 RegisterVersionServiceServer。
- [客户端](../test/grpc_version/main.go)：deadline、状态错误和连接关闭示例。

复制模板后不需要版本 RPC 时，删除 RegisterVersionServiceServer 调用及对应导入，再删除 `api/version`、`controller/rpc`、`test/grpc_version` 和版本 RPC 专用测试。标准 Health 服务不依赖版本 RPC。

## 修改协议后重新生成

已提交生成文件，普通构建无需 protoc。修改 `.proto` 才需要生成工具。
固定版本：protoc 33.0、protoc-gen-go v1.36.12、protoc-gen-go-grpc v1.6.0。
工具来自 [Protobuf 官方发布](https://github.com/protocolbuffers/protobuf/releases/tag/v33.0)和 [gRPC Go 官方工具链](https://grpc.io/docs/languages/go/quickstart/)。

将官方 protoc 解压至 `tmp/grpc-tools/protoc`，然后在 PowerShell 执行（GOBIN 仅临时改变）：

```powershell
$previousGobin = $env:GOBIN
try {
    $env:GOBIN = Join-Path (Get-Location) 'tmp/grpc-tools'
    go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.0
} finally {
    $env:GOBIN = $previousGobin
}
./tmp/grpc-tools/protoc/bin/protoc.exe `
  --plugin=protoc-gen-go=tmp/grpc-tools/protoc-gen-go.exe `
  --plugin=protoc-gen-go-grpc=tmp/grpc-tools/protoc-gen-go-grpc.exe `
  --go_out=. --go_opt=paths=source_relative `
  --go-grpc_out=. --go-grpc_opt=paths=source_relative `
  api/version/v1/version.proto
```

生成文件与 `.proto` 一起提交；不要手写消息描述符或修改生成代码。
