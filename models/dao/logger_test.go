package dao

import (
	"context"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormlog "gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"serv_tpl/internal/requestctx"
	"serv_tpl/pkg/logger"
	"testing"
	"time"
)

func TestSQLLoggerRecordsErrorWithoutFormattingSQL(t *testing.T) {
	dir := t.TempDir()
	h, err := logger.NewFileLogger(filepath.Join(dir, "sql.log"), "debug")
	require.NoError(t, err)
	logger.SetGlobal(h)
	t.Cleanup(func() { _ = h.Close(); fallback, _ := logger.NewStdLogger("info"); logger.SetGlobal(fallback) })
	ctx := requestctx.WithTrace(context.Background(), "0123456789abcdef")
	secret := "password-secret token-secret 13800138000 SELECT-private db.private:3306"
	fmtSQL := func() (string, int64) { t.Fatal("不应格式化 SQL"); return secret, 1 }
	l := sqlLogger{level: gormlog.Warn}
	for _, level := range []gormlog.LogLevel{gormlog.Silent, gormlog.Error, gormlog.Warn, gormlog.Info} {
		current := l.LogMode(level)
		current.Info(ctx, secret)
		current.Warn(ctx, secret)
		current.Error(ctx, secret)
		current.Trace(ctx, time.Now(), fmtSQL, nil)
		current.Trace(ctx, time.Now().Add(-time.Second), fmtSQL, nil)
		current.Trace(ctx, time.Now(), fmtSQL, &mysql.MySQLError{Number: 1062, Message: secret})
	}
	require.NoError(t, h.Logger().Sync())
	files, err := filepath.Glob(filepath.Join(dir, "sql-*.log"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.Contains(t, string(data), `"error":"Error 1062: `+secret+`"`)
	require.Contains(t, string(data), `"trace_id":"0123456789abcdef"`)
	require.Contains(t, string(data), "数据库慢操作")
	require.Contains(t, string(data), "数据库操作")
}
