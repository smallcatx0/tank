package valid

import (
	"gtank/middleware/resp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

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

type UserModify struct {
	User     string `json:"user"`
	Pass     string `json:"pass"`
	Nickname string `json:"nickname"`
	Truename string `json:"truename"`
	Email    string `json:"email"`
}

type JWTData struct {
	Uid      string `json:"uid,omitempty"`
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
