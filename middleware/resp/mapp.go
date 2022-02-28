package resp

const (
	// http 状态码枚举
	HttpOk      = 200
	HttpErr     = 500
	HttpFail    = 400
	HttpNoLogin = 401
	HttpIllegal = 403
)

var (
	// errcode枚举
	Code_Succ = 0
	Code_Fail = 1
	Code_Err  = 999

	Code_Cal = 31000
	Code_Mdb = 32000
	Code_Rdb = 33000

	Code_ParamInValid = 10000
	Code_NoLogin      = 10004
	Code_LoginTimeout = 10005
	Code_IllegalToken = 10006
	Code_Illegal      = 40003
)

var ErrNos = map[int]string{
	Code_Succ: "操作成功",
	Code_Fail: "参数错误",
	Code_Err:  "系统错误,请稍后再试",
	Code_Mdb:  "mysql 连接错误",
}

var (
	// 参数错误
	ParamInValid = func(msg string) *Exception {
		return NewException(HttpFail, Code_ParamInValid, msg)
	}
	ErrMysql     = NewException(HttpErr, Code_Mdb)
	ErrRedis     = NewException(HttpErr, Code_Rdb)
	NoLogin      = NewException(HttpNoLogin, Code_NoLogin, "未登录")
	LoginTimeOut = NewException(HttpNoLogin, Code_LoginTimeout, "登录超时")
	IllegalToken = NewException(HttpNoLogin, Code_IllegalToken, "token非法")
	Illegal      = NewException(HttpIllegal, Code_Illegal, "非法操作")
)
