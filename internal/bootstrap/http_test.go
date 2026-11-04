package bootstrap

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func httpOpts() HTTPOptions {
	return HTTPOptions{Address: "127.0.0.1:0", ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, DrainTimeout: time.Second}
}
func TestHTTPListenAndDrain(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, err := StartHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte("finished"))
	}), httpOpts())
	require.NoError(t, err)
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
	s, e := StartHTTP(http.NotFoundHandler(), httpOpts())
	require.NoError(t, e)
	defer s.Shutdown(context.Background())
	o := httpOpts()
	o.Address = s.listener.Addr().String()
	_, e = StartHTTP(http.NotFoundHandler(), o)
	require.Error(t, e)
}
func TestHTTPForcedCancel(t *testing.T) {
	entered, finished := make(chan struct{}), make(chan struct{})
	o := httpOpts()
	o.DrainTimeout = 20 * time.Millisecond
	s, e := StartHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(finished) }), o)
	require.NoError(t, e)
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
		require.NoError(t, s.Shutdown(context.Background()))
	}
}
func TestWatchHTTPFailure(t *testing.T) {
	code := run(make(chan os.Signal), func(int) { t.Error("forced") }, func(ctx context.Context, r *Runtime) error {
		ln, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			return e
		}
		r.WatchHTTP(serveHTTP(http.NotFoundHandler(), httpOpts(), brokenListener{Listener: ln}))
		return nil
	})
	require.Equal(t, 1, code)
}
func TestConcurrentShutdown(t *testing.T) {
	s, e := StartHTTP(http.NotFoundHandler(), httpOpts())
	require.NoError(t, e)
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
