package mask

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"a", "****"},
		{"ab", "a****"},
		{"中文测试", "中****"}, // 多字节安全
	}
	for _, c := range cases {
		assert.Equal(t, c.want, MaskString(c.in), "input=%q", c.in)
	}
}

func TestPhone(t *testing.T) {
	assert.Equal(t, "138****5678", Phone("13800005678"))
	// 非手机号形态回退通用打码，不泄露原串
	assert.Equal(t, "1****", Phone("123"))
	assert.Equal(t, "2****", Phone("23800005678")) // 非 1 开头
	assert.Equal(t, "1****", Phone("138000a5678")) // 含非数字
}

func TestEmail(t *testing.T) {
	assert.Equal(t, "t****@corp.com", Email("test@corp.com"))
	assert.Equal(t, "n****", Email("no-at-sign"))
	assert.Equal(t, "@corp.com"[:1]+"****", MaskString("@corp.com")) // 本地部分为空 → 通用打码
	assert.Equal(t, "x****", Email("x@y"))                           // 域名无点 → 通用打码
}

func TestIDCard(t *testing.T) {
	assert.Equal(t, "110****34", IDCard("110101199003070534"))
	assert.Equal(t, "110****54", IDCard("110101900307054")) // 15 位一代
	assert.Equal(t, "1****", IDCard("12345"))               // 过短回退
}

func TestTokenSecretPassword(t *testing.T) {
	assert.Equal(t, "****", Token("abcdefg"))
	assert.Equal(t, "", Token(""))
	assert.Equal(t, "****", Secret("hmac-secret"))
	assert.Equal(t, "****", Password("p@ssw0rd"))
}

// TestNoLeakage 安全边界：任何输出都不得包含原串主体。
func TestNoLeakage(t *testing.T) {
	secret := "13800005678"
	for _, v := range []string{Phone(secret), MaskString(secret), Token(secret), Secret(secret)} {
		assert.False(t, strings.Contains(v, "8000056"), "masked value leaks body: %q", v)
	}
}

func BenchmarkMaskString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = MaskString("some-sensitive-value-0123456789")
	}
}

func BenchmarkPhone(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Phone("13800005678")
	}
}
