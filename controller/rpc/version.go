// Package rpc 提供 gRPC 请求处理，包括服务版本查询。
package rpc

import (
	"context"

	versionv1 "serv_tpl/sdk/version/v1"
	"serv_tpl/internal/global"

	"google.golang.org/grpc/status"
)

// VersionService 提供服务版本查询，与 HTTP 根路径使用相同配置来源。
type VersionService struct {
	versionv1.UnimplementedVersionServiceServer
}

// Version 返回服务名称、描述及版本，未配置的字段返回空字符串。
func (VersionService) Version(ctx context.Context, _ *versionv1.VersionRequest) (*versionv1.VersionReply, error) {
	// 步骤1：遵循调用方取消或超时。
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	// 步骤2：与 HTTP 根路径保持键位一致，不返回其他配置。
	return &versionv1.VersionReply{
		Name:    global.AppConf.GetString("name"),
		Desc:    global.AppConf.GetString("desc"),
		Version: global.AppConf.GetString("ver"),
	}, nil
}
