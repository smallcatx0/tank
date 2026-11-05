package controller

import (
	"fmt"
	"serv_tpl/bootstrap"
	"serv_tpl/internal/resp"
	"serv_tpl/models/dao"

	"github.com/gin-gonic/gin"
)

// AppHealth master 版健康/运维控制器；与 min_tpl 的 Health（healthz.go）并存，
// 保留 Ready 之外的热重载配置与调试能力，最终取舍由人工裁决
type AppHealth struct{}

func (AppHealth) Healthz(c *gin.Context) {
	resp.Succ(c, "")
}

func (AppHealth) Ready(c *gin.Context) {
	resp.Succ(c, resp.ErrNos[resp.Code_Succ])
}

// 重新加载配置文件
func (AppHealth) ReloadConf(c *gin.Context) {
	bootstrap.InitConf(&bootstrap.Param.C)
	bootstrap.InitLog()
	bootstrap.InitDB()
	resp.Succ(c, "")
}

func (AppHealth) Test(c *gin.Context) {
	// resp.Fail(c, resp.ParamInValid("错了"))
	// sql 错误
	table, _ := c.GetQuery("table")
	dao.MysqlCli.Exec(fmt.Sprintf("select * from `%s`", table))
	resp.Fail(c, resp.NewException(401, 10000, "4011111"))
}
