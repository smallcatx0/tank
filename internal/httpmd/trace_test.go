package httpmd

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"serv_tpl/internal/requestctx"
	"strings"
	"testing"
)

func TestTraceContract(t *testing.T) {
	r := gin.New()
	r.Use(Trace)
	r.GET("/", func(c *gin.Context) {
		require.Equal(t, c.GetString(RequestIDKey), requestctx.TraceID(c.Request.Context()))
		require.GreaterOrEqual(t, requestctx.LatencyMS(c.Request.Context()), float64(0))
		c.String(200, c.GetString(RequestIDKey))
	})
	for _, id := range []string{"0123456789ABCDEF", "0123456789ABCDEF0123456789ABCDEF", "", "0000000000000000", strings.Repeat("0", 32), "not-a-trace-id!!!", strings.Repeat("g", 16), "1", strings.Repeat("a", 33)} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set(RequestIDHeader, id)
		req.Header.Set("request_id", "legacy-id")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		got := w.Body.String()
		require.True(t, validTraceID(got))
		require.Equal(t, got, w.Header().Get(RequestIDHeader))
		if validTraceID(id) {
			require.Equal(t, strings.ToLower(id), got)
		} else {
			require.Len(t, got, 32)
			require.NotEqual(t, id, got)
		}
	}
}

func TestTraceConcurrent(t *testing.T) {
	r := gin.New()
	r.Use(Trace)
	r.GET("/", func(c *gin.Context) { c.String(200, requestctx.TraceID(c.Request.Context())) })
	for _, ch := range "123456789abcdef" {
		id := strings.Repeat(string(ch), 32)
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(RequestIDHeader, id)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, id, w.Body.String())
		})
	}
}

func BenchmarkTrace(b *testing.B) {
	r := gin.New()
	r.Use(Trace)
	r.GET("/", func(c *gin.Context) { c.Status(204) })
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}
}
