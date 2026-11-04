# 协议定义

本目录只存放 `.proto` 契约及生成的 Go 消息/服务接口，不包含业务实现。生成文件保留工具英文注释，禁止手工修改。

## 版本协议生成

当前生成文件记录的工具版本：protoc 6.33.0、protoc-gen-go 1.36.12、protoc-gen-go-grpc 1.6.0。准备对应工具后，在仓库根执行：

```powershell
protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/version/v1/version.proto
```

启动、注册及调用方式见 [业务接入指南](../业务接入指南.md)，客户端示例见 [版本查询客户端](../test/grpc_version/main.go)。
