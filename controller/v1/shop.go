package v1

import (
	"context"
	"serv_tpl/internal/global"
	"serv_tpl/internal/requestctx"
	"serv_tpl/internal/resp"
	"serv_tpl/models/dao/schema"
	"serv_tpl/models/valid"
	"serv_tpl/pkg/logger"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var defaultTimeout = time.Second

// Shop 提供默认开启的店铺示例，使用全局数据库连接
type Shop struct{}

// Create 校验参数并真正写入店铺，成功后返回新记录。
func (Shop) Create(c *gin.Context) {
	// 步骤1：校验参数，业务期限从请求 context 派生。
	p := valid.ShopAdd{}
	if err := valid.BindJsonAndCheck(c, &p); err != nil {
		resp.Fail(c, err)
		return
	}

	// 业务逻辑
	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultTimeout)
	defer cancel()
	shop := schema.Shop{Name: p.Name, Status: p.Status}
	if err := shop.Create(ctx, global.MysqlCli); err != nil {
		resp.Fail(c, err)
		return
	}
	logger.L().Info("创建店铺", zap.Uint64("shop_id", shop.ID), zap.String("status", shop.Status),
		zap.String("trace_id", requestctx.TraceID(ctx)), zap.Float64("latency_ms", requestctx.LatencyMS(ctx)))

	resp.Succ(c, shop)
}

// List 按状态筛选并返回稳定排序的分页结果。
func (Shop) List(c *gin.Context) {
	p := struct {
		Status string `form:"status" binding:"omitempty,oneof=enable disable"`
	}{}
	if err := valid.BindQueryAndCheck(c, &p); err != nil {
		resp.Fail(c, err)
		return
	}
	// 步骤1：Count 与 Find 共用一次操作期限。
	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultTimeout)
	defer cancel()
	pg := resp.NewPage(c)
	list, err := schema.ListShops(ctx, global.MysqlCli, p.Status, pg)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	logger.L().Info("查询店铺", zap.String("status", p.Status), zap.Int("page", pg.Page), zap.Int("limit", pg.Limit),
		zap.String("trace_id", requestctx.TraceID(ctx)), zap.Float64("latency_ms", requestctx.LatencyMS(ctx)))
	resp.Paginate(c, pg, list)
}
