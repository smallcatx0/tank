package httpmd

import (
	"gtank/middleware/resp"
	"gtank/valid"
	"strings"

	"github.com/gin-gonic/gin"
)

func JwtAuth() func(c *gin.Context) {

	return func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("Authorization"))
		if token == "" {
			resp.Fail(c, resp.NoLogin)
			c.Abort()
			return
		}
		data, err := valid.JWTPase(token)
		if err != nil {
			resp.Fail(c, err)
			c.Abort()
			return
		}
		c.Set("jwtinfo", data.JWTData)
	}
}
