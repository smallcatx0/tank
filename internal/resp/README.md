# internal/response 包使用说明

## 包概述

`response` 包（`serv_tpl/internal/response`）是 HTTP 接口的**统一响应与错误处理包**，提供三类能力：

1. **统一响应格式**：所有接口出口固定为 `{errcode, msg, data, request_id}` JSON 结构；
2. **异常体系**：`Exception` 类型 + 预置错误码/错误对象，业务错误与系统错误自动区分处理；
3. **分页工具**：`Pagination` 与 gorm 集成的查询分页、页码计算。

放在 `internal/` 下是刻意的：本包只服务本仓库的接口层，不对外复用，因此也没有额外套 `pkg/` 中间层（见 `项目规范.md` 第 9 节）。

## 文件结构

| 文件 | 说明 |
| --- | --- |
| `resp.go` | 响应体结构、`Succ` / `Fail` / `Paginate` / `Response` / `SuccJsonRaw` 出口函数 |
| `excep.go` | `Exception` 错误类型、`NewSucc` / `NewException` 构造器 |
| `mapp.go` | 错误码枚举、预置异常对象与 `ParamInValid` 工厂函数 |
| `paginate.go` | `Pagination` 分页结构及 gorm 分页查询 |

## 一、统一响应格式

```json
{
    "errcode": 0,
    "msg": "操作成功",
    "data": { },
    "request_id": "xxx"
}
```

HTTP 状态码与 `errcode` 绑定：参数错误为 `400 + 10000`，未登录为 `401 + 10004`，成功为 `200 + 0`。

### 出口函数

| 函数 | 说明 |
| --- | --- |
| `Succ(c *gin.Context, data interface{})` | 成功响应，`errcode=0`，msg 固定"操作成功" |
| `Fail(c *gin.Context, err error)` | 失败响应，透传 err 给 `Response` 统一处理 |
| `Paginate(c *gin.Context, pg *Pagination, list interface{})` | 分页成功响应，`data` 为 `{"page": pg, "list": list}` |
| `SuccJsonRaw(c *gin.Context, data string)` | 原始 JSON 透传，HTTP 200、errcode=0；request_id 经 JSON 转义 |
| `Response(c *gin.Context, err error)` | 底层统一出口，一般不直接调用 |

`SuccJsonRaw` 将 data 原样嵌入响应，不解析、重新序列化或校验。调用方必须保证 data 是完整合法的 JSON 值（对象、数组或标量均可）；无数据时传入 `"null"`，不能传空字符串。

`Response` 的错误处理规则：

- `err` 为 `*Exception` / `Exception`，或通过 `%w` 包装这些业务异常 → 按其 `HTTPCode`、`ErrCode`、`Msg`、`Data` 原样返回；
- nil error 或匹配到空 `*Exception` → 返回 `500 + errcode 50000`、msg 为"服务错误"；成功请显式调用 `Succ`；
- 其他普通 error（如 gorm 报错）→ 返回 `500 + errcode 50000`；**dev 环境**（`internal/global.Env() == "dev"`）回显真实错误信息便于调试，**生产环境** msg 统一为"服务错误"并记 error 日志。

## 二、异常体系（Exception）

```go
type Exception struct {
    HTTPCode int         // HTTP 状态码
    ErrCode  int         // 业务错误码
    Msg      string      // 提示信息
    Data     interface{} // 附加数据
    Warp     []error     // 预留字段，当前未使用，也未实现 Unwrap
}
```

实现了 `Error() string`（返回 Msg），因此可直接作为 `error` 在函数间传递、被 `Fail` 识别。

`Response` 使用 `errors.As` 识别单层或多层包装，优先匹配 `*Exception`，再匹配 `Exception`；`%v` 仅格式化文本，不保留错误链。未匹配业务异常的普通错误在 dev 下仍返回完整错误原文（含包装上下文）。

### 构造器与预置错误码

| 错误码 | 常量 | 含义 |
| --- | --- | --- |
| 0 | `Code_Succ` | 成功 |
| 1 | `Code_Fail` | 参数错误 |
| 999 | `Code_Err` | 系统错误 |
| 10000 | `Code_ParamInValid` | 请求参数校验失败（HTTP 400） |
| 10004 / 10005 / 10006 | `Code_NoLogin` / `Code_LoginTimeout` / `Code_IllegalToken` | 未登录 / 登录超时 / token 非法（HTTP 401） |
| 31000 / 32000 / 33000 | `Code_Cal` / `Code_Mdb` / `Code_Rdb` | 验证码 / MySQL / Redis 错误 |
| 40003 | `Code_Illegal` | 非法操作（HTTP 403） |

`ErrNos` 为错误码 → 默认文案的映射表（当前仅登记部分码值）。

预置异常对象：`ErrMysql`、`ErrRedis`、`NoLogin`、`LoginTimeOut`、`IllegalToken`、`Illegal`；`ParamInValid(msg ...string)` 是**工厂函数**，每次生成新实例。

## 三、分页（Pagination）

| 函数/方法 | 说明 |
| --- | --- |
| `NewPage(c *gin.Context) *Pagination` | 从 query 参数 `page`（默认 1）、`limit`（默认 10）构造 |
| `NewPagination(page, limit int) *Pagination` | 手工构造；`page<1` 归一为 1，`limit` 归一到 `[1, MAX_LIMIT]` |
| `(*Pagination) Calc(total int)` | 按总数计算 `TotalPage`/`Offset`，页码钳制到 `[1, TotalPage]` |
| `(*Pagination) Paginate(tx *gorm.DB) (*gorm.DB, error)` | 先 `Count` 再附加 `Offset/Limit` |

## 四、控制器标准用法

```go
func (h Demo) List(c *gin.Context) {
    p := valid.DemoListParam{}
    if err := valid.BindQueryAndCheck(c, &p); err != nil {
        response.Fail(c, err) // 校验错误本身就是 *Exception
        return
    }

    q := dao.MysqlCli.Model(&rds.Demo{})
    pg := response.NewPage(c)
    q, err := pg.Paginate(q)
    if err != nil {
        response.Fail(c, err)
        return
    }
    if pg.Total == 0 {
        response.Paginate(c, pg, nil) // 空列表短路
        return
    }
    list := make([]rds.Demo, 0, pg.Limit)
    if err := q.Find(&list).Error; err != nil {
        response.Fail(c, err)
        return
    }
    response.Paginate(c, pg, list)
}
```

## 五、request_id 与链路追踪

- key 为常量 `RequestIDKey = "x-b3-traceid"`；`internal/httpmd` 中定义了同名常量，改键名须同步两处。
- 响应体中的 `request_id` 取自 `c.GetString(RequestIDKey)`。
- **现状**：仓库尚未提供写入该 key 的中间件（`internal/httpmd/trace.go` 为空文件），因此该字段当前恒为空串，需先补齐 Trace 中间件才会生效。

## 六、注意事项

- **`Fail` 后必须 `return`**：`Response` 已写入响应，继续执行会造成重复写入或逻辑错误；
- **预置异常是包级共享单例**（`NoLogin`、`ErrMysql` 等）：只可直接使用，**不要修改其字段**（如 `Data`），否则会跨请求串数据；需要携带数据时用 `NewException` 新建实例；
- dev/生产的错误信息策略依赖 `internal/global.Env()`，新增环境值时注意同步；
- 新增错误码时须在 `mapp.go` 登记常量与 `ErrNos` 文案，并同步本文件。
