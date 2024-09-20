package dbstrategy

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ZapWriter struct{ *zap.Logger }

func (w *ZapWriter) Printf(tpl string, args ...interface{}) {
	tpl = strings.ReplaceAll(tpl, "\n", " ")
	msg := "[sql] " + fmt.Sprintf(tpl, args...)
	if _, ok := args[1].(error); ok {
		w.Error(msg)
	} else {
		w.Info(msg)
	}
}

// 获取临时数据库链接
func connDb(dsn string, l *zap.Logger) (*gorm.DB, error) {
	logger := logger.New(
		&ZapWriter{l},
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      logger.Warn,
			Colorful:      false,

			IgnoreRecordNotFoundError: true,
		},
	)
	conn, err := gorm.Open(
		mysql.Open(dsn),
		&gorm.Config{
			Logger: logger,
		},
	)
	if err != nil {
		return nil, err
	}
	mdb, err := conn.DB()
	if err != nil {
		return nil, err
	}
	err = mdb.Ping()
	if err != nil {
		return nil, err
	}
	// 优化参数

	return conn, nil
}

func CloseDb(db *gorm.DB) {
	mdb, err := db.DB()
	if err != nil {
		return
	}
	mdb.Close()
}

// 数据库链接脱敏
func DsnMask(dsn string) string {
	info := strings.SplitN(dsn, "@", 2)
	u := strings.SplitN(info[0], ":", 2)
	return u[0] + ":******@" + info[1]
}
