# serv_tpl

Go 服务脚手架，提供 HTTP API、可选 gRPC、MySQL/GORM、Redis、zap 日志和进程退出管理。

## 本地启动

需要 Go 1.26（见 [go.mod](go.mod)）、MySQL 和 Redis。以下命令在仓库根目录使用 Bash执行：

```bash
# conf/app.yaml 不存在时复制；已有配置请直接编辑，不要覆盖
test -f conf/app.yaml || cp conf/app.yaml.example conf/app.yaml
```

编辑 MySQL DSN 与 Redis 地址/密码，确保数据库存在。服务不自动建库建表。

```bash
go run ./cmd/api -config conf/app.yaml
```

API 默认监听 HTTP 8080，gRPC 默认关闭。在另一个终端检查：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/ready
curl -fsS http://localhost:8080/
```

`/healthz` 表示进程存活，`/ready` 表示 HTTP 生命周期就绪，不持续探测下游。HTTP 和 gRPC 业务接入、协议生成与调用步骤见 [业务接入指南](业务接入指南.md)。

运行 Kafka 示例 worker：先复制配置模板，按目标测试环境修改 brokers、topics 和 group_id，并预先创建主题。

```bash
test -f conf/worker.yaml || cp conf/worker.yaml.example conf/worker.yaml
# 编辑 conf/worker.yaml 后启动
go run ./cmd/sth_worker -config conf/worker.yaml
```

worker 初始化日志、可选系统心跳和可选 Kafka 消费者，可按配置启动独立监控 HTTP，不启动业务 HTTP/gRPC，不连接 MySQL/Redis。kafka.enable 默认 false，worker 示例模板显式开启。示例回调仅记录消息位置，成功后提交位点；请使用独立测试主题与消费者组。消费失败触发非零退出，收到退出信号后先排空消费者、最后关闭日志。配置与业务回调替换方式见 [Kafka 组件说明](pkg/kafka/README.md#worker-入口装配)。

## 配置与日志

配置键、单位与行为见 [配置模板](conf/app.yaml.example)。使用 `-config` 选择 YAML；当前没有环境变量覆盖配置机制。

- `env: dev` 保留普通系统错误回显，其他环境隐藏响应细节；生产部署显式设置 `env: prod`。未配置 env 时默认 dev。
- 系统错误日志直接使用 `zap.Error(err)` 保留原文，详见 [日志说明](pkg/logger/README.md)和[响应契约](internal/resp/README.md)。
- 容器推荐 `log.type: stdout`；文件模式需要可写目录，按天切割且固定保留 365 天。未知日志类型使用 stdout，未知等级回退 info。
- `metrics.enable` 仅启用系统状态日志。Prometheus 与追踪使用独立的 `observability` 配置，默认关闭，观测故障降级不阻止业务；启用、抓取和链路验证见 [观测接入](internal/telemetry/README.md)。
- API 在启动阶段直接装配 MySQL/Redis，不提供组件配置开关。不需要时裁剪 [API 入口](cmd/api/main.go)与[存储装配](internal/bootstrap/bootstrap.go)，并移除依赖该组件的业务和路由。

## 构建与检查

```bash
go build ./cmd/...
go test ./...
go vet ./...
```

完整竞态和信号退出验证在具备 C 编译器的 Linux 环境执行 `go test -race ./...`；安装 Staticcheck 后执行 `staticcheck ./...`。

存储/入口集成测试需设置专用隔离环境变量 `D02_TEST_MYSQL_DSN`、`D02_TEST_MYSQL_ADDR`、`D02_TEST_REDIS_ADDR`，不得指向业务库。未配置时相关用例跳过。测试要求见 [项目规范](项目规范.md)。常规测试可复用常驻测试依赖，但必须隔离并清理本次测试数据；认证、断网和重启等故障专项仍使用独立临时实例。

## 容器构建与运行

[Dockerfile](Dockerfile) 使用 Go 多阶段构建，固定构建 API。后续需要构建 worker 镜像时，新增独立 Dockerfile。运行镜像为 Alpine 3.24，提供 `/bin/sh` 和基础命令，包含静态程序和时区数据，使用 UID/GID 65532；不包含配置，从构建镜像复制公共 CA 信任库，私有 CA 由部署侧注入。

```bash
docker build -t serv-tpl:api .
```

需要其他模块代理时传 `--build-arg GOPROXY=https://goproxy.cn,direct`。构建使用已提交的 protobuf Go 文件，无需 protoc。

准备宿主机 `conf/app.yaml`，MySQL/Redis 地址需能从容器访问：同一 Docker 网络使用服务名，Docker Desktop 可用 `host.docker.internal` 访问宿主机。容器内 `localhost` 指容器自身。

```bash
appConfig="$(pwd)/conf/app.yaml"
docker run -d --name serv-tpl-api --stop-timeout 35 -p 8080:8080 --mount "type=bind,source=$appConfig,target=/app/conf/app.yaml,readonly" serv-tpl:api
docker logs serv-tpl-api
# 进入运行中的容器进行排查，退出 shell 后再停止服务
docker exec -it serv-tpl-api /bin/sh
docker stop --timeout 35 serv-tpl-api
```

开启 gRPC 时将 `grpc.address` 改为 `0.0.0.0:9090`，在运行参数增加 `-p 9090:9090`；仅发布端口不会开启服务。当前为明文 RPC，跨信任边界的 TLS/认证由实际项目配置。

挂载配置需允许 UID 65532 读取；文件日志目录需允许其写入。Docker 停止等待时间应大于 `shutdown_timeout`，示例分别为 35 秒和 30 秒。

## 目录与复用

| 目录 | 职责 |
| --- | --- |
| `cmd/` | API、worker 入口装配 |
| `routes/`、`controller/` | 路由注册、HTTP/RPC 业务处理 |
| `api/` | protobuf 契约与生成代码 |
| `models/dao/`、`models/dao/schema/` | 连接管理、GORM 模型及数据方法 |
| `models/valid/` | HTTP 参数绑定与校验 |
| `internal/` | 启动、配置、响应、中间件、请求 context |
| `pkg/` | 日志、脱敏等通用工具 |

复制为新服务后更新模块名、Go import、`.proto` 的 `go_package` 并重新生成协议；修改服务信息和连接地址，按需裁剪业务与组件。开发流程见 [AGENTS.md](AGENTS.md)，现状见 [curr.md](curr.md)，计划见 [plan.md](plan.md)。
