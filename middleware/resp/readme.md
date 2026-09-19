# middleware/resp 包使用说明

## 包概述

`resp` 包（`gtank/middleware/resp`）是 HTTP 接口的**统一响应与错误处理包**，提供三类能力：

1. **统一响应格式**：所有接口出口固定为 `{errcode, msg, data, request_id}` JSON 结构；
2. **异常体系**：`Exception` 类型 + 预置错误码/错误对象，业务错误与系统错误自动区分处理；
3. **分页工具**：`Pagination` 与 gorm 集成的查询分页、页码计算。

控制器、参数校验包（`models/valid`）、中间件均依赖本包返回错误，是项目接口规范的落地核心（规范见 `doc/规范.md`、`doc/接口文档.md`）。

## 文件结构

| 文件 | 说明 |
| --- | --- |
| `resp.go` | 响应体结构、`Succ` / `Fail` / `Paginate` / `Response` / `SuccJsonRaw` 出口函数 |
| `excep.go` | `Exception` 错误类型、`NewSucc` / `NewException` 构造器 |
| `mapp.go` | 错误码枚举、预置异常对象与 `ParamInValid` 工厂函数 |
| `paginate.go` | `Pagination` 分页结构及 gorm 分页查询 |

## 一、统一响应格式

所有出口响应均为：

```json
{
    "errcode": 0,          // 业务错误码，0 表示成功
    "msg": "操作成功",      // 提示信息
    "data": { ... },       // 业务数据，无则为 null
    "request_id": "xxx"    // 链路追踪 id（x-b3-traceid）
}
```

HTTP 状态码与 `errcode` 绑定：如参数错误为 `400 + 10000`，未登录为 `401 + 10004`，成功为 `200 + 0`。

### 出口函数

| 函数 | 说明 |
| --- | --- |
| `Succ(c *gin.Context, data interface{})` | 成功响应，`errcode=0`，msg 固定"操作成功" |
| `Fail(c *gin.Context, err error)` | 失败响应，透传 err 给 `Response` 统一处理 |
| `Paginate(c *gin.Context, pg *Pagination, list interface{})` | 分页成功响应，`data` 为 `{"page": pg, "list": list}` |
| `SuccJsonRaw(c *gin.Context, data string)` | data 已是 JSON 字符串时的零序列化直出（高性能场景） |
| `Response(c *gin.Context, err error)` | 底层统一出口，一般不直接调用 |

`Response` 的错误处理规则：

- `err` 为 `*Exception` / `Exception` → 按其 `HTTPCode`、`ErrCode`、`Msg`、`Data` 原样返回；
- 其他普通 error（如 gorm 报错）→ 返回 `500 + errcode 50000`；**dev 环境**（`conf.Env() == "dev"`）回显真实错误信息便于调试，**生产环境** msg 统一为"服务错误"并记 error 日志，避免泄漏内部细节。

## 二、异常体系（Exception）

```go
type Exception struct {
    HTTPCode int         // HTTP 状态码
    ErrCode  int         // 业务错误码
    Msg      string      // 提示信息
    Data     interface{} // 附加数据
    Warp     []error     // 预留的错误包装字段（当前未使用）
}
```

实现了 `Error() string`（返回 Msg），因此可直接作为 `error` 在函数间传递、被 `Fail` 识别。

### 构造器

| 函数 | 说明 |
| --- | --- |
| `NewSucc(data interface{}) *Exception` | 构造成功响应（200 / errcode 0） |
| `NewException(httpcode, errcode int, msg ...string) *Exception` | 构造任意异常；不传 msg 时默认"服务错误"，多个 msg 以空格拼接 |

### 预置错误码（mapp.go）

| 错误码 | 常量 | 含义 |
| --- | --- | --- |
| 0 | `Code_Succ` | 成功 |
| 1 | `Code_Fail` | 参数错误 |
| 999 | `Code_Err` | 系统错误 |
| 10000 | `Code_ParamInValid` | 请求参数校验失败（HTTP 400） |
| 10004 | `Code_NoLogin` | 未登录（HTTP 401） |
| 10005 | `Code_LoginTimeout` | 登录超时（HTTP 401） |
| 10006 | `Code_IllegalToken` | token 非法（HTTP 401） |
| 31000 / 32000 / 33000 | `Code_Cal` / `Code_Mdb` / `Code_Rdb` | 验证码 / MySQL / Redis 错误 |
| 40003 | `Code_Illegal` | 非法操作（HTTP 403） |

`ErrNos` 为错误码 → 默认文案的映射表。

### 预置异常对象

| 对象 | 等价于 | 使用场景 |
| --- | --- | --- |
| `ParamInValid(msg ...string)` | 工厂**函数**，每次生成新实例 | 参数校验失败，携带自定义提示（`valid` 包大量使用） |
| `NoLogin` | 401 / 10004 "未登录" | 未携带/无法解析 token |
| `LoginTimeOut` | 401 / 10005 "登录超时" | token 过期（`JWTParse` 返回） |
| `IllegalToken` | 401 / 10006 "token非法" | token 签名错误等 |
| `Illegal` | 403 / 40003 "非法操作" | 越权等操作 |
| `ErrMysql` / `ErrRedis` | 500 / 32000、33000 | 依赖组件故障 |

## 三、分页（Pagination）

```go
type Pagination struct {
    Page      int `json:"page"`       // 当前页（从 1 开始）
    Limit     int `json:"limit"`      // 每页条数
    Total     int `json:"total"`      // 总条数
    TotalPage int `json:"total_page"` // 总页数
    Offset    int `json:"-"`          // 计算出的偏移量，不输出到响应
}
```

### API

| 函数/方法 | 说明 |
| --- | --- |
| `NewPage(c *gin.Context) *Pagination` | 从 query 参数 `page`（默认 1）、`limit`（默认 10）构造 |
| `NewPagination(page, limit int) *Pagination` | 手工构造；`page<1` 归一为 1，`limit` 归一到 `[1, MAX_LIMIT]`（0/负数取默认 `DEF_LIMIT=10`，超 `MAX_LIMIT=1000` 截断） |
| `(*Pagination) Calc(total int)` | 按总数计算 `TotalPage`/`Offset`；页码钳制到 `[1, TotalPage]`（`TotalPage=0` 时保持 1），`Offset` 恒非负 |
| `(*Pagination) Paginate(tx *gorm.DB) (*gorm.DB, error)` | 对 gorm 查询先 `Count` 再附加 `Offset/Limit`，一步完成 |

> 注：`limit < 1` 与 `total=0` 的边界已修复归一（见第六节），`Calc` 不会再触发除零 panic，`Offset` 也不会为负。

## 四、控制器标准用法

完整流程示例（参考 `controller/v1/admin.go` 的 List）：

```go
func (UserAdmin) List(c *gin.Context) {
    p := valid.UserListParam{}
    if err := valid.BindQueryAndCheck(c, &p); err != nil {
        resp.Fail(c, err) // 校验错误本身就是 *Exception
        return
    }

    q := dao.MysqlCli.Model(&rds.User{})
    // ... 拼接 where 条件 ...

    pg := resp.NewPage(c)
    q, err := pg.Paginate(q) // count + offset/limit
    if err != nil {
        resp.Fail(c, err)    // 普通 error → 500，生产环境隐藏细节
        return
    }
    if pg.Total == 0 {
        resp.Paginate(c, pg, nil) // 空列表短路，不再查明细
        return
    }
    users := make([]rds.User, 0, pg.Limit)
    if err := q.Find(&users).Error; err != nil {
        resp.Fail(c, err)
        return
    }
    resp.Paginate(c, pg, users)
}
```

成功响应：

```go
resp.Succ(c, gin.H{"auth": token})
```

业务错误的两种写法：

```go
// 1. 带自定义文案的参数错误（前端可见）
resp.Fail(c, resp.ParamInValid("手机号已经存在"))

// 2. 预置异常对象
resp.Fail(c, resp.NoLogin)
```

## 五、request_id 与链路追踪

- key 为常量 `RequestIDKey = "x-b3-traceid"`；
- 入口中间件（`middleware/httpmd/http.go`）为每个请求生成/复用 traceid，同时 `c.Set`（供进程内读取）并 `c.Header`（写入响应头）；
- 响应体中的 `request_id` 取自**请求头**的 `x-b3-traceid`（即上游调用方传入的值），便于跨服务串联日志。

## 六、注意事项与已知问题

- **`Fail` 后必须 `return`**：`Response` 调用 `c.JSON` 已写入响应，继续执行会造成重复写入或逻辑错误；
- **预置异常是包级共享单例**（`NoLogin`、`ErrMysql` 等）：只可直接使用，**不要修改其字段**（如 `Data`），否则会跨请求串数据；需要携带数据时用 `NewException` 新建实例；
- dev/生产的错误信息策略依赖 `gtank/internal/conf` 的 `Env()`，新增环境值时注意同步；
- 错误码新增流程：在 `mapp.go` 中登记 code 常量与预置异常，并同步更新 `doc/接口文档.md`。
