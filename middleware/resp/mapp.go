package resp

const (
	// http 状态码枚举
	HttpOk   = 200
	HttpErr  = 500
	HttpFail = 400

	// errcode枚举
	Code_Succ = 0
	Code_Fail = 1
	Code_Err  = 999

	Code_Cal = 31000
	Code_Mdb = 32000
	Code_Rdb = 33000

	Code_ParamInValid = 10000
)

var ErrNos = map[int]string{
	Code_Succ: "操作成功",
	Code_Fail: "参数错误",
	Code_Err:  "系统错误,请稍后再试",
	Code_Mdb:  "mysql 连接错误",

	Code_ParamInValid: "参数错误，请检查后重试",
}

var (
	ErrMysql = NewException(HttpErr, Code_Mdb)
	ErrRedis = NewException(HttpErr, Code_Rdb)
)

var (
	// 参数错误
	ParamInValid = func(msg string) *Exception {
		return NewException(401, Code_ParamInValid, msg)
	}
)
