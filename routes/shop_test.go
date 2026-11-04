package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShopCreate(t *testing.T) {
	r := gin.New()
	RegisterHTTP(r, func() bool { return true })
	for _, tc := range []struct {
		name, body string
		code       int
	}{
		{"valid", `{"name":"中心店","status":"enable"}`, 200},
		{"disabled", `{"name":"西区店","status":"disable"}`, 200},
		{"malformed", `{`, 400},
		{"missing", `{}`, 400},
		{"blank name", `{"name":"   ","status":"enable"}`, 400},
		{"invalid status", `{"name":"中心店","status":"unknown"}`, 400},
		{"long name", `{"name":"` + strings.Repeat("店", 51) + `","status":"enable"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/admin/shop/create", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, tc.code, w.Code)
			var body struct {
				Errcode int                            `json:"errcode"`
				Data    struct{ Demo, Validated bool } `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			if tc.code == 200 {
				require.Zero(t, body.Errcode)
				require.True(t, body.Data.Demo)
				require.True(t, body.Data.Validated)
			} else {
				require.Equal(t, 10000, body.Errcode)
			}
		})
	}
}

func TestShopList(t *testing.T) {
	r := gin.New()
	RegisterHTTP(r, func() bool { return true })
	for _, tc := range []struct {
		query              string
		page, limit, total int
		ids                []int
	}{
		{"", 1, 10, 3, []int{1, 2, 3}},
		{"?page=2&limit=1", 2, 1, 3, []int{2}},
		{"?page=99&limit=2", 2, 2, 3, []int{3}},
		{"?status=enable", 1, 10, 2, []int{1, 2}},
		{"?status=disable", 1, 10, 1, []int{3}},
		{"?page=-1&limit=0", 1, 10, 3, []int{1, 2, 3}},
		{"?page=9223372036854775807&limit=1001", 1, 1000, 3, []int{1, 2, 3}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/shop/list"+tc.query, nil))
			require.Equal(t, 200, w.Code)
			var body struct {
				Errcode int `json:"errcode"`
				Data    struct {
					Page struct{ Page, Limit, Total int }
					List []struct{ ID int }
				}
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Zero(t, body.Errcode)
			require.Equal(t, tc.page, body.Data.Page.Page)
			require.Equal(t, tc.limit, body.Data.Page.Limit)
			require.Equal(t, tc.total, body.Data.Page.Total)
			ids := make([]int, 0, len(body.Data.List))
			for _, item := range body.Data.List {
				ids = append(ids, item.ID)
			}
			require.Equal(t, tc.ids, ids)
		})
	}
}

func TestShopRoutesAndInvalidQuery(t *testing.T) {
	r := gin.New()
	RegisterHTTP(r, func() bool { return true })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/shop/list?status=unknown", nil))
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "查询参数无效")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/user/list", nil))
	require.Equal(t, 404, w.Code)
}
