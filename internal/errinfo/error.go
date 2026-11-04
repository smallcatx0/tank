// Package errinfo 提取错误的安全诊断字段，不输出第三方错误原文。
package errinfo

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// Fields 返回类型、取消类别及数据库错误编号，不读取错误消息中的用户数据。
func Fields(err error) []zap.Field {
	category := "system"
	if errors.Is(err, context.DeadlineExceeded) {
		category = "timeout"
	}
	if errors.Is(err, context.Canceled) {
		category = "canceled"
	}
	fields := []zap.Field{zap.String("error_type", fmt.Sprintf("%T", err)), zap.String("error_category", category)}
	var dbErr *mysql.MySQLError
	if errors.As(err, &dbErr) && dbErr != nil {
		fields = append(fields, zap.Uint16("mysql_code", dbErr.Number))
	}
	return fields
}
