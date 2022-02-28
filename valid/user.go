package valid

import (
	"gtank/middleware/resp"
	"gtank/models/dao/cal"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

const (
	// 手机号 正则
	Rule_phone = "^((13[0-9])|(14[5,7])|(15[0-3,5-9])|(17[0,3,5-8])|(18[0-9])|166|198|199|(147))\\d{8}$"
	Rule_email = `\w[-\w.+]*@([A-Za-z0-9][-A-Za-z0-9]+\.)+[A-Za-z]{2,14}`
)

type PhoneReg struct {
	Phone string `json:"phone" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

func (p *PhoneReg) Valid() error {
	reg := regexp.MustCompile(Rule_phone)
	if !reg.MatchString(p.Phone) {
		return resp.ParamInValid("手机号格式错误")
	}
	// 校验验证码
	if !cal.SMSVCode(p.Phone, p.Code) {
		return resp.ParamInValid("验证码错误")
	}
	return nil
}

type UserLogin struct {
	User string `json:"user" binding:"required"`
	Pass string `json:"pass" binding:"required"`
}

func (u *UserLogin) Valid() error {
	if u.User == "admin" {
		return resp.ParamInValid("不允许使用该用户名")
	}
	return nil
}

type ModPass struct {
	Type    string `json:"type" binding:"required"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Code    string `json:"code"`
	OldPass string `json:"old_pass"`
	Pass    string `json:"pass" binding:"required"`
}

func (p *ModPass) Valid() error {
	switch strings.ToLower(p.Type) {
	case "pass":
		if p.OldPass == "" {
			return resp.ParamInValid("旧密码，不能为空")
		}
	case "phone":
		reg := regexp.MustCompile(Rule_phone)
		if !reg.MatchString(p.Phone) {
			return resp.ParamInValid("手机号格式错误")
		}
		// 校验验证码
		if !cal.SMSVCode(p.Phone, p.Code) {
			return resp.ParamInValid("验证码错误")
		}
	case "email":
		reg := regexp.MustCompile(Rule_email)
		if !reg.MatchString(p.Email) {
			return resp.ParamInValid("邮箱格式错误")
		}
		// 校验验证码
		if !cal.EmailCode(p.Email, p.Code) {
			return resp.ParamInValid("验证码错误")
		}
	default:
		return resp.ParamInValid("类型错误")
	}
	return nil
}

// token中携带的数据
type JWTData struct {
	Uid      int    `json:"uid,omitempty"`
	User     string `json:"user,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Truename string `json:"rname,omitempty"`
	Nickname string `json:"name,omitempty"`
}

type Claim struct {
	*jwt.StandardClaims
	JWTData
}

func (j *JWTData) Generate() (string, error) {
	t := jwt.New(jwt.SigningMethodHS256)
	c := &Claim{
		StandardClaims: &jwt.StandardClaims{},
	}
	// 设置过期时间 3600s
	c.ExpiresAt = time.Now().Add(5 * time.Minute).Unix()
	c.JWTData = *j
	t.Claims = c
	return t.SignedString([]byte("sk"))
}

func JWTPase(token string) (*Claim, error) {
	t, err := jwt.ParseWithClaims(token, &Claim{}, func(t *jwt.Token) (interface{}, error) {
		return []byte("sk"), nil
	})
	if err != nil {
		if e, ok := err.(jwt.ValidationError); ok {
			if e.Errors&jwt.ValidationErrorExpired != 0 {
				return nil, resp.LoginTimeOut
			} else {
				return nil, resp.IllegalToken
			}
		}
		return nil, err
	}
	data, ok := t.Claims.(*Claim)
	if ok && t.Valid {
		return data, nil
	}
	return nil, resp.IllegalToken
}

// 获取jwt中的用户信息
func UserInfo(c *gin.Context) (*JWTData, bool) {
	data, ok := c.Get("jwtinfo")
	if !ok {
		// 解析
		return UserInfoPase(c)
	}
	ret, ok := data.(JWTData)
	return &ret, ok
}

func UserInfoPase(c *gin.Context) (*JWTData, bool) {
	tokenStr := strings.TrimSpace(c.GetHeader("Authorization"))
	if tokenStr == "" {
		return nil, false
	}
	raw, err := JWTPase(tokenStr)
	if err != nil {
		return nil, false
	}
	c.Set("jwtinfo", raw.JWTData)
	return &raw.JWTData, true
}
