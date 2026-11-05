# models/valid 包使用说明

## 包概述

`valid` 包（`gtank/models/valid`）是请求参数校验与用户身份相关的数据结构包，主要提供三类能力：

1. **参数绑定与校验**：封装 gin 的 JSON / Query 绑定，并支持自定义校验规则；
2. **请求参数结构体定义**：各接口（用户、后台、MQ）的请求参数模型；
3. **JWT 令牌处理**：token 的生成、解析，以及从请求上下文中获取当前登录用户信息。

## 文件结构

| 文件 | 说明 |
| --- | --- |
| `valid.go` | 绑定+校验的通用函数，`CustomValidor` 接口定义 |
| `user.go` | 用户端参数模型、正则规则、JWT 生成/解析、当前用户获取 |

## 一、参数绑定与校验

### 核心机制

```go
type CustomValidor interface {
    Valid() error
}
```

绑定流程分两步：

1. gin 自动绑定并按结构体 tag（`binding:"required"` 等）做基础校验，失败则返回 `resp.ParamInValid` 包装的错误；
2. 若参数类型实现了 `Valid() error` 方法，则继续执行自定义校验。

### 提供的函数

| 函数 | 说明 |
| --- | --- |
| `BindJsonAndCheck(c *gin.Context, param interface{}) error` | 绑定 JSON 请求体并触发校验 |
| `BindQueryAndCheck(c *gin.Context, param interface{}) error` | 绑定 URL Query 参数并触发校验 |

### 使用示例

```go
param := valid.PhoneReg{}
err := valid.BindJsonAndCheck(c, &param)
if err != nil {
    resp.Fail(c, err)
    return
}
```

如需自定义校验，只要为参数结构体实现指针方法 `Valid() error`：

```go
func (p *PhoneReg) Valid() error {
    reg := regexp.MustCompile(Rule_phone)
    if !reg.MatchString(p.Phone) {
        return resp.ParamInValid("手机号格式错误")
    }
    return nil
}
```

> 注意：`Valid()` 必须定义在**指针接收者**上，且调用时传入的是 `&param`，否则不会触发自定义校验。

## 二、JWT 令牌

### 相关类型

```go
// token 中携带的用户数据
type JWTData struct {
    Uid      int    `json:"uid,omitempty"`
    User     string `json:"user,omitempty"`
    Phone    string `json:"phone,omitempty"`
    Truename string `json:"rname,omitempty"`
    Nickname string `json:"name,omitempty"`
}

// 完整 Claim（StandardClaims + JWTData）
type Claim struct {
    *jwt.StandardClaims
    JWTData
}
```

### 提供的函数/方法

| 函数 | 说明 |
| --- | --- |
| `(*JWTData) Generate() (string, error)` | 生成 HS256 签名 token，有效期 **1 小时** |
| `JWTParse(token string) (*Claim, error)` | 解析 token；过期返回 `resp.LoginTimeOut`，非法返回 `resp.IllegalToken` |
| `UserInfo(c *gin.Context) (*JWTData, bool)` | 获取当前登录用户信息，解析失败返回 `false` |
| `UserInfoParse(c *gin.Context) (*JWTData, bool)` | 从 `Authorization` 头解析用户信息（`UserInfo` 内部兜底调用） |

### 生成 token（登录成功后）

```go
data := valid.JWTData{Uid: u.Id, User: u.User}
token, err := data.Generate()
```

### 获取当前用户（需登录的接口）

```go
t, ok := valid.UserInfo(c)
if !ok {
    resp.Fail(c, resp.NoLogin)
    return
}
// t.Uid / t.User ...
```

`UserInfo` 先读取上下文缓存，未命中时从请求头 `Authorization` 解析并回写缓存。

> 说明：缓存 key 已统一为 `"jwtinfo"`（`JwtAuth` 中间件、`UserInfo` 读取、`UserInfoParse` 回写三处保持一致），中间件鉴权后接口内首次调用 `UserInfo` 即可直接命中缓存，不会重复解析 token。

## 四、新增参数模型的推荐做法

1. 在对应业务文件（或新建文件）中定义结构体，JSON 接口用 `json` tag，Query 接口用 `form` tag；
2. 必填字段加 `binding:"required"`；
3. 有格式/业务校验需求时，实现 `Valid() error`，校验错误统一使用 `resp.ParamInValid(...)` 返回；
4. 控制器中通过 `BindJsonAndCheck` / `BindQueryAndCheck` 绑定，出错时 `resp.Fail(c, err)` 后直接 return。

## 五、注意事项

- 错误返回统一走 `gtank/middleware/resp` 包，便于中间件/响应层统一处理；
