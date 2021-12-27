package resp

import "strings"

type Exception struct {
	HTTPCode int
	ErrCode  int
	Msg      string
	Data     interface{}
	Warp     []error
}

func (e Exception) Error() string {
	return e.Msg
}

func NewSucc(data interface{}) *Exception {
	return &Exception{
		HTTPCode: 200,
		ErrCode:  0,
		Msg:      "操作成功",
		Data:     data,
	}
}

func NewException(httpcode, errcode int, msg ...string) *Exception {
	e := &Exception{
		HTTPCode: httpcode,
		ErrCode:  errcode,
	}
	if len(msg) == 0 {
		e.Msg = "服务错误"
	} else {
		e.Msg = strings.Join(msg, " ")
	}
	return e
}
