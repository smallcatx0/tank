package v1

import (
	"gitee.com/smallcatx0/gtank/models/page"
	"gitee.com/smallcatx0/gtank/valid"
	"github.com/gin-gonic/gin"
)

func LoginByPwd(c *gin.Context) {
	param := valid.UserLogin{}
	err := valid.BindAndCheck(c, &param)
	if err != nil {
		r.Fail(c, err)
		return
	}
	res := new(page.User).LoginByPwd()
	r.Succ(c, res)
}
