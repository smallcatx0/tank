package httpmd

import (
	"encoding/json"
	"gtank/middleware/resp"
	"gtank/valid"
	"net/url"
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
		if err == nil {
			// 将解析信息写入header
			raw, _ := json.Marshal(data.JWTData)
			p := url.QueryEscape(string(raw))
			c.Request.Header.Add("xu-info", p)
			c.Set("xu-info", data.JWTData)
			return
		}
		resp.Fail(c, err)
		c.Abort()

	}
}
