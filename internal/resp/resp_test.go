package resp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"serv_tpl/internal/global"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestSuccJsonRaw(t *testing.T) {
	for _, data := range []string{
		" \n{\"large\":9007199254740993, \"text\":\"\\u4e2d<>&%s\"}\t ",
		`[1, "two", null]`, `"a\\b\"c"`, `1.2300e+10`, `true`, `null`,
	} {
		for _, requestID := range []string{"", "request-123", "引号\"反斜杠\\换行\n\t\r\x00\x1f<>&\u2028\u2029"} {
			t.Run(data+"/"+requestID, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Set(RequestIDKey, requestID)
				SuccJsonRaw(c, data)
				if w.Code != 200 || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
					t.Fatalf("status=%d headers=%v", w.Code, w.Header())
				}
				var got struct {
					ErrCode   int             `json:"errcode"`
					Msg       string          `json:"msg"`
					Data      json.RawMessage `json:"data"`
					RequestID string          `json:"request_id"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.ErrCode != 0 || got.Msg != "操作成功" || got.RequestID != requestID {
					t.Fatalf("unexpected response: %+v", got)
				}
				// 检查原始字节（含外围空白），防止透传被替换为重新序列化。
				if !strings.Contains(w.Body.String(), `"data":`+data+`,"request_id":`) {
					t.Fatalf("data changed: %s", w.Body.String())
				}
			})
		}
	}
}

func TestResponseErrorContract(t *testing.T) {
	previous := global.AppConf
	global.AppConf = viper.New()
	t.Cleanup(func() { global.AppConf = previous })

	var nilException *Exception
	for _, env := range []string{"dev", "prod"} {
		t.Run(env, func(t *testing.T) {
			global.AppConf.Set("env", env)
			internalMessage := "internal error details"
			wantMessage := "服务错误"
			if env == "dev" {
				wantMessage = internalMessage
			}
			for _, tc := range []struct {
				name         string
				err          error
				status, code int
				message      string
			}{
				{"nil", nil, 500, 50000, "服务错误"},
				{"nil exception", nilException, 500, 50000, "服务错误"},
				{"wrapped nil exception", fmt.Errorf("context: %w", nilException), 500, 50000, "服务错误"},
				{"ordinary error", errors.New(internalMessage), 500, 50000, wantMessage},
				{"business pointer", NewException(400, 40001, "参数错误"), 400, 40001, "参数错误"},
				{"business value", *NewException(400, 40001, "参数错误"), 400, 40001, "参数错误"},
				{"wrapped business pointer", fmt.Errorf("context: %w", NewException(409, 40901, "状态冲突")), 409, 40901, "状态冲突"},
				{"wrapped business value", fmt.Errorf("context: %w", *NewException(422, 42201, "参数不符合规则")), 422, 42201, "参数不符合规则"},
				{"nested business pointer", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", NewException(409, 40901, "状态冲突"))), 409, 40901, "状态冲突"},
				{"nested business value", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", *NewException(422, 42201, "参数不符合规则"))), 422, 42201, "参数不符合规则"},
				{"success", NewSucc(nil), 200, 0, "操作成功"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					w := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(w)
					c.Request = httptest.NewRequest("GET", "/test", nil)
					c.Set(RequestIDKey, "test-request")
					// 直接调用，不挂 Recovery，确保空错误不会依靠中间件吞掉 panic。
					Fail(c, tc.err)
					var got body
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if w.Code != tc.status || got.ErrCode != tc.code || got.Msg != tc.message || got.Data != nil || got.RequestID != "test-request" {
						t.Fatalf("status=%d body=%+v", w.Code, got)
					}
				})
			}
		})
	}
}

func TestResponseWrappedDataAndOrdinaryError(t *testing.T) {
	previous := global.AppConf
	global.AppConf = viper.New()
	t.Cleanup(func() { global.AppConf = previous })
	business := NewException(409, 40901, "状态冲突")
	business.Data = "current-state"
	ordinary := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", errors.New("details")))
	for _, env := range []string{"dev", "prod"} {
		global.AppConf.Set("env", env)
		message := "服务错误"
		if env == "dev" {
			message = ordinary.Error()
		}
		for _, tc := range []struct {
			name   string
			err    error
			status int
			want   body
		}{
			{"pointer data", fmt.Errorf("context: %w", business), 409, body{ErrCode: 40901, Msg: "状态冲突", Data: "current-state"}},
			{"value data", fmt.Errorf("context: %w", *business), 409, body{ErrCode: 40901, Msg: "状态冲突", Data: "current-state"}},
			{"ordinary", ordinary, 500, body{ErrCode: 50000, Msg: message}},
		} {
			t.Run(env+"/"+tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("GET", "/test", nil)
				Response(c, tc.err)
				var got body
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if w.Code != tc.status || got != tc.want {
					t.Fatalf("status=%d body=%+v; want status=%d body=%+v", w.Code, got, tc.status, tc.want)
				}
			})
		}
	}
}
