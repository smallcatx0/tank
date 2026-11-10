package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"serv_tpl/internal/global"

	"github.com/gin-gonic/gin"

	"sync"
	"testing"
	"time"
)

func httpOpts() httpOptions {
	return httpOptions{ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, DrainTimeout: time.Second}
}

func TestMustStartHTTPAssembly(t *testing.T) {
	oldMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(oldMode) })
	for _, debug := range []bool{false, true} {
		func() {
			// 随机本地端口，不初始化或访问 MySQL/Redis。
			s := MustStartHTTP(global.LifecycleConfig{Port: 0, Debug: debug, Drain: time.Second}, closedRuntime(t))
			defer s.Shutdown(context.Background())
			wantMode := gin.ReleaseMode
			if debug {
				wantMode = gin.DebugMode
			}
			require.Equal(t, wantMode, gin.Mode())
			request := func(path string) int {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.Header.Set("x-b3-traceid", "0123456789abcdef")
				s.server.Handler.ServeHTTP(w, req)
				if path != "/reload" {
					var body struct {
						RequestID string `json:"request_id"`
					}
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
					require.Equal(t, "0123456789abcdef", body.RequestID)
				}
				return w.Code
			}
			require.Equal(t, http.StatusOK, request("/healthz"))
			require.Equal(t, http.StatusOK, request("/ready"))
			require.Equal(t, http.StatusNotFound, request("/reload"))
			require.NoError(t, s.Shutdown(context.Background()))
			require.Equal(t, http.StatusServiceUnavailable, request("/ready"))
		}()
	}
}
func TestHTTPListenAndDrain(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := serveHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte("finished"))
	}), httpOpts(), ln)
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	body := make(chan string, 1)
	async(t, func() {
		resp, e := client.Get("http://" + s.listener.Addr().String())
		if e != nil {
			body <- e.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		body <- string(b)
	})
	<-entered
	closed := make(chan error, 1)
	async(t, func() { closed <- s.Shutdown(context.Background()) })
	require.Eventually(t, func() bool {
		c, e := net.DialTimeout("tcp", s.listener.Addr().String(), 20*time.Millisecond)
		if e == nil {
			c.Close()
		}
		return e != nil
	}, time.Second, time.Millisecond)
	select {
	case <-closed:
		t.Fatal("did not wait for active request")
	default:
	}
	close(release)
	require.Equal(t, "finished", <-body)
	require.NoError(t, <-closed)
	require.NoError(t, <-s.Done())
	require.NoError(t, s.Shutdown(context.Background()))
}
func TestHTTPConflict(t *testing.T) {
	oldMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(oldMode) })
	ln, e := net.Listen("tcp", ":0")
	require.NoError(t, e)
	defer ln.Close()
	require.PanicsWithValue(t, "HTTP 监听失败", func() {
		MustStartHTTP(global.LifecycleConfig{Port: ln.Addr().(*net.TCPAddr).Port, Drain: time.Second}, closedRuntime(t))
	})
}
func TestHTTPForcedCancel(t *testing.T) {
	entered, finished := make(chan struct{}), make(chan struct{})
	o := httpOpts()
	o.DrainTimeout = 20 * time.Millisecond
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	s := serveHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(finished) }), o, ln)
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	requestDone := make(chan struct{})
	async(t, func() {
		defer close(requestDone)
		resp, _ := client.Get("http://" + s.listener.Addr().String())
		if resp != nil {
			resp.Body.Close()
		}
	})
	<-entered
	require.ErrorIs(t, s.Shutdown(context.Background()), context.DeadlineExceeded)
	<-finished
	<-requestDone
}

type brokenListener struct {
	net.Listener
	panics bool
}

func (l brokenListener) Accept() (net.Conn, error) {
	if l.panics {
		panic("secret")
	}
	return nil, errors.New("broken")
}
func TestHTTPServeFailure(t *testing.T) {
	for _, panics := range []bool{false, true} {
		ln, e := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, e)
		s := serveHTTP(http.NotFoundHandler(), httpOpts(), brokenListener{ln, panics})
		require.Error(t, <-s.Done())
		require.Error(t, s.Shutdown(context.Background()), "退出也须报告 Serve 故障")
	}
}
func TestConcurrentShutdown(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	s := serveHTTP(http.NotFoundHandler(), httpOpts(), ln)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		async(t, func() {
			defer wg.Done()
			if err := s.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}
