package v1

import (
	"github.com/gin-gonic/gin"
	"gtank/middleware/resp"
	"gtank/models/page"
	"gtank/valid"
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
