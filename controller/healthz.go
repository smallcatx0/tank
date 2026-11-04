package controller

import (
	"github.com/gin-gonic/gin"
	"serv_tpl/internal/resp"
)

// Health 分离存活和生命周期就绪，不承诺下游持续可用。
type Health struct{ IsReady func() bool }

// Healthz 返回进程存活信息。
func (Health) Healthz(c *gin.Context) { resp.Succ(c, "健康检查正常") }

// Ready 返回启动完成且未开始关闭的状态。
func (h Health) Ready(c *gin.Context) {
	if h.IsReady == nil || !h.IsReady() {
		resp.Fail(c, resp.NewException(503, 50300, "服务未就绪"))
		return
	}
	resp.Succ(c, resp.ErrNos[resp.Code_Succ])
}
