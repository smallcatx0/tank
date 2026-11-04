package errinfo

import (
	"context"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"testing"
)

func TestSafeFields(t *testing.T) {
	for _, tc := range []struct {
		err      error
		category string
	}{
		{fmt.Errorf("secret: %w", context.Canceled), "canceled"},
		{context.DeadlineExceeded, "timeout"},
		{&mysql.MySQLError{Number: 1062, Message: "password=secret token=secret"}, "system"},
	} {
		enc := zapcore.NewMapObjectEncoder()
		for _, f := range Fields(tc.err) {
			f.AddTo(enc)
		}
		require.Equal(t, tc.category, enc.Fields["error_category"])
		require.NotContains(t, fmt.Sprint(enc.Fields), "secret")
		if tc.category == "system" {
			require.Equal(t, uint16(1062), enc.Fields["mysql_code"])
		}
	}
}
