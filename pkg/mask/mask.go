// Package mask 提供项目统一的敏感字段脱敏能力（项目规范第 2 节）。
//
// 存在理由：规范强制「脱敏必须在构造错误之前完成，默认统一使用项目内脱敏工具函数，
// 同一仓库内不得长期存在多套脱敏实现」。本包是唯一收敛目标：
// error 构造、日志打点、配置回显等一切敏感字段输出均经此包处理。
//
// 覆盖字段：手机号、邮箱、密码、token、证件号。
package mask

import (
	"regexp"
	"strings"

	"go.uber.org/zap"
)

// masked 定长掩码：不复现原串长度，避免通过长度泄露信息。
const masked = "****"

// emailPattern 简单邮箱形态（够用于脱敏判定，不做 RFC 校验）。
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// MaskString 通用打码：保留首字符，其余以定长掩码替代；空串返回空串。
// 用于形态未知的敏感值，宁可多打不可漏打。
func MaskString(s string) string {
	// 步骤1：空与单字符直接全掩，避免"保留首字符即全量"的退化
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 1 {
		return masked
	}
	return string(r[0]) + masked
}

// Phone 手机号脱敏：保留前 3 后 4（如 138****5678）；形态不符时回退通用打码。
func Phone(s string) string {
	if !isMainlandMobile(s) {
		return MaskString(s)
	}
	return s[:3] + masked + s[len(s)-4:]
}

// Email 邮箱脱敏：保留首字符与域名（如 t***@corp.com）；形态不符时回退通用打码。
func Email(s string) string {
	at := strings.Index(s, "@")
	if at <= 0 || !emailPattern.MatchString(s) {
		return MaskString(s)
	}
	return s[:1] + masked + s[at:]
}

// IDCard 证件号脱敏：保留前 3 后 2；形态不符时回退通用打码。
func IDCard(s string) string {
	r := []rune(s)
	if len(r) < 15 { // 一代 15 位、二代 18 位均视为合法下限
		return MaskString(s)
	}
	return string(r[:3]) + masked + string(r[len(r)-2:])
}

// Token 固定 token 脱敏：一律全掩。
//
// 决策：token 用于日志与 error 上下文时，任何前缀保留都无调试价值且扩大泄露面，
// 故不做部分打码；需要"区分是哪个 token"时请用调用方自带的标识字段（如 header 名）。
func Token(s string) string {
	if s == "" {
		return ""
	}
	return masked
}

// Secret 签名密钥脱敏：同 Token，一律全掩。
func Secret(s string) string {
	return Token(s)
}

// Password 密码脱敏：密码任何形态都不得出现在输出中，一律全掩。
func Password(s string) string {
	return Token(s)
}

// TokenField 构造已脱敏的 zap 字段，供日志打点统一使用。
func TokenField(key, v string) zap.Field { return zap.String(key, Token(v)) }

// SecretField 构造已脱敏的 zap 字段，供日志打点统一使用。
func SecretField(key, v string) zap.Field { return zap.String(key, Secret(v)) }

// isMainlandMobile 判定 11 位大陆手机号形态（1 开头）。
func isMainlandMobile(s string) bool {
	if len(s) != 11 || s[0] != '1' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
