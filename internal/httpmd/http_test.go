package httpmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"serv_tpl/internal/global"
	"serv_tpl/internal/resp"
	"serv_tpl/pkg/logger"
	"strings"
	"testing"
)

func TestRequestLogsAndErrorBoundaries(t *testing.T) {
	old := global.AppConf
	global.AppConf = viper.New()
	t.Cleanup(func() { global.AppConf = old })
	dir := t.TempDir()
	h, err := logger.NewFileLogger(filepath.Join(dir, "capture.log"), "debug")
	require.NoError(t, err)
	logger.SetGlobal(h)
	t.Cleanup(func() { _ = h.Close(); fallback, _ := logger.NewStdLogger("info"); logger.SetGlobal(fallback) })
	r := gin.New()
	r.Use(Trace, ReqLog, Recovery)
	secret := "password-secret token-secret 13800138000 SELECT-private db.private:3306"
	r.POST("/echo", func(c *gin.Context) {
		body, e := io.ReadAll(c.Request.Body)
		require.NoError(t, e)
		resp.Succ(c, string(body))
	})
	r.GET("/error", func(c *gin.Context) { resp.Fail(c, fmt.Errorf("查询店铺: %w", errors.New(secret))) })
	r.GET("/panic", func(*gin.Context) { panic(secret) })
	r.GET("/invalid", func(c *gin.Context) { resp.Fail(c, resp.ParamInValid("请求参数无效")) })
	for _, env := range []string{"dev", "prod"} {
		global.AppConf.Set("env", env)
		for _, path := range []string{"/echo", "/error", "/panic", "/invalid"} {
			method := "GET"
			if path == "/echo" {
				method = "POST"
			}
			req := httptest.NewRequest(method, path+"?token=token-secret", strings.NewReader(secret))
			req.Header.Set("Authorization", "Bearer token-secret")
			req.Header.Set("Cookie", "password-secret")
			req.Header.Set(RequestIDHeader, "0123456789abcdef")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, "0123456789abcdef", body["request_id"])
			switch path {
			case "/echo":
				require.Equal(t, secret, body["data"])
			case "/error":
				require.Equal(t, 500, w.Code)
				if env == "dev" {
					require.Equal(t, "查询店铺: "+secret, body["msg"])
				} else {
					require.Equal(t, "服务错误", body["msg"])
				}
			case "/panic":
				require.Equal(t, 500, w.Code)
				require.Equal(t, "服务错误", body["msg"])
			case "/invalid":
				require.Equal(t, 400, w.Code)
			}
		}
	}
	require.NoError(t, h.Logger().Sync())
	files, err := filepath.Glob(filepath.Join(dir, "capture-*.log"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	for _, value := range strings.Fields(secret) {
		require.NotContains(t, string(data), value)
	}
	var completed int
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		require.Equal(t, "0123456789abcdef", entry["trace_id"])
		require.Contains(t, entry, "latency_ms")
		if entry["msg"] == "HTTP 请求完成" {
			completed++
			require.NotContains(t, entry["route"], "?")
		}
	}
	require.Equal(t, 8, completed)
}
