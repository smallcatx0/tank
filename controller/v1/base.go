package v1

import (
	"gitee.com/smallcatx0/gtank/middleware/httpmd"

	"github.com/gin-gonic/gin"
)

var r = new(httpmd.Resp)

func Demo(c *gin.Context) {
	r.Succ(c, "demo")
}
