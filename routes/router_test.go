package routes

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"serv_tpl/internal/httpmd"
	"sync/atomic"
	"testing"
)

func TestRoutesAndRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var ready atomic.Bool
	r := gin.New()
	r.Use(httpmd.Recovery)
	Register(r, ready.Load)
	r.GET("/panic", func(*gin.Context) { panic("secret-token") })
	r.GET("/partial", func(c *gin.Context) { c.String(200, "partial"); panic("secret") })
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	require.Equal(t, 503, request("/ready").Code)
	ready.Store(true)
	require.Equal(t, 200, request("/ready").Code)
	ready.Store(false)
	require.Equal(t, 503, request("/ready").Code)
	w := request("/healthz")
	require.Equal(t, 200, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, float64(0), body["errcode"])
	require.Equal(t, 404, request("/reload").Code)
	w = request("/panic")
	require.Equal(t, 500, w.Code)
	require.NotContains(t, w.Body.String(), "secret")
	require.Equal(t, "partial", request("/partial").Body.String())
	require.Equal(t, 200, request("/healthz").Code)
	require.Equal(t, 200, request("/").Code)
}
