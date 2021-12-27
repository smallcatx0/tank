package v1

import (
	"gitee.com/smallcatx0/gtank/middleware/resp"
	"gitee.com/smallcatx0/gtank/models/page"
	"gitee.com/smallcatx0/gtank/valid"
	"github.com/gin-gonic/gin"
)

func LoginByPwd(c *gin.Context) {
	param := valid.UserLogin{}
	err := valid.BindAndCheck(c, &param)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	res := new(page.User).LoginByPwd()
	resp.Succ(c, res)
}
