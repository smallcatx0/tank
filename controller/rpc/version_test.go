package rpc

import (
	"context"
	"testing"
	"time"

	versionv1 "serv_tpl/api/version/v1"
	"serv_tpl/internal/global"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVersion(t *testing.T) {
	previous := global.AppConf
	t.Cleanup(func() { global.AppConf = previous })
	global.AppConf = viper.New()
	resp, err := (VersionService{}).Version(context.Background(), &versionv1.VersionRequest{})
	require.NoError(t, err)
	require.Empty(t, resp.GetName())
	require.Empty(t, resp.GetDesc())
	require.Empty(t, resp.GetVersion())
	global.AppConf.Set("name", "app")
	global.AppConf.Set("desc", "应用程序配置")
	global.AppConf.Set("ver", "1.2.3")
	global.AppConf.Set("version", "不得使用此键")
	resp, err = (VersionService{}).Version(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "app", resp.GetName())
	require.Equal(t, "应用程序配置", resp.GetDesc())
	require.Equal(t, "1.2.3", resp.GetVersion())
}

func TestVersionCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (VersionService{}).Version(ctx, &versionv1.VersionRequest{})
	require.Equal(t, codes.Canceled, status.Code(err))
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = (VersionService{}).Version(ctx, &versionv1.VersionRequest{})
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))
}
