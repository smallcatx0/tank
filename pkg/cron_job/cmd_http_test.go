package cronjob

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func Test_MakeHttpCmd(t *testing.T) {
	var (
		cfg = CommandHttp{
			Retry:  3,
			Method: "POST",
			Url:    "http://postman-echo.com/post",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"User-Agent":   "cronjob",
			},
			Body: `{"key":"value"}`,
		}
	)
	fn, err := MakeCommandHttp(&cfg, zap.NewExample().With(zap.String("cron_name", "postman-echo")))
	assert.NoError(t, err)
	fn()
}

func Test_MakeHttpCdm_dial_fail(t *testing.T) {
	var (
		cfg = CommandHttp{
			Retry:  3,
			Method: "POST",
			Url:    "http://ther.nothost.com/post",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"User-Agent":   "cronjob",
			},
			Body: `{"key":"value"}`,
		}
	)
	fn, err := MakeCommandHttp(&cfg, zap.NewExample().With(zap.String("cron_name", "dial_fail")))
	assert.NoError(t, err)
	fn()
}
