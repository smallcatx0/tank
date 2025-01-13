package cronjob

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

var (
	httpCli = &http.Client{
		Timeout: 5 * time.Second,
	}
	SuccCode = map[int]struct{}{
		http.StatusOK:        {},
		http.StatusCreated:   {},
		http.StatusAccepted:  {},
		http.StatusNoContent: {},
	}
)

type CommandHttp struct {
	Retry   int
	Method  string
	Url     string
	Headers map[string]string
	Body    string
}

// 构造一个 http请求udf
func MakeCommandHttp(cmd *CommandHttp, logger *zap.Logger) (func(), error) {
	var retry = 3
	if cmd.Retry > 0 {
		retry = cmd.Retry
	}
	req, err := http.NewRequest(
		cmd.Method,
		cmd.Url,
		strings.NewReader(cmd.Body),
	)
	if err != nil {
		return nil, err
	}
	for k, v := range cmd.Headers {
		req.Header.Set(k, v)
	}
	logFields := []zap.Field{
		zap.String("url", cmd.Method+" "+cmd.Url),
		zap.String("body", cmd.Body),
		zap.Any("headers", cmd.Headers),
	}
	return func() {
		var resp *http.Response
		var err error
		for i := 0; i < retry; i++ {
			st := time.Now()
			resp, err = httpCli.Do(req)
			dt := time.Since(st)
			log := append(logFields, zap.Duration("const", dt))
			if err != nil {
				log = append(log, zap.Error(err))
				logger.Error(fmt.Sprintf("[http_command] 请求失败-%d", i), log...)
				time.Sleep(200 * time.Millisecond)
				continue
			}
			log = append(log, zap.Int("status_code", resp.StatusCode))
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				log = append(log, zap.Error(err))
			} else {
				log = append(log, zap.String("resp_body", string(respBody)))
			}
			if _, ok := SuccCode[resp.StatusCode]; ok {
				logger.Info("[http_command] 请求成功", log...)
				break
			}
			logger.Error(fmt.Sprintf("[http_command] 请求失败-%d", i), log...)
		}
	}, nil
}
