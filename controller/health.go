package controller

import (
	"fmt"
	"gtank/bootstrap"
	"gtank/middleware/resp"
	"gtank/models/dao"

	"github.com/gin-gonic/gin"
)

type Health struct{}

func (Health) Healthz(c *gin.Context) {
	resp.Succ(c, "")
}

func (Health) Ready(c *gin.Context) {
	resp.Succ(c, resp.ErrNos[resp.Code_Succ])
}

// 重新加载配置文件
func (Health) ReloadConf(c *gin.Context) {
	bootstrap.InitConf(&bootstrap.Param.C)
	bootstrap.InitLog()
	bootstrap.InitDB()
	resp.Succ(c, "")
}

func (Health) Test(c *gin.Context) {
	// resp.Fail(c, resp.ParamInValid("错了"))
	// sql 错误
	table, _ := c.GetQuery("table")
	dao.MDB.Exec(fmt.Sprintf("select * from `%s`", table))
	resp.Fail(c, resp.NewException(401, 10000, "4011111"))
}
