package valid

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type checkedParam struct {
	Value int `json:"value" form:"value"`
}

func (p *checkedParam) Valid() error {
	if p.Value == 7 {
		return errors.New("custom validation")
	}
	return nil
}

func TestBinders(t *testing.T) {
	for _, query := range []bool{false, true} {
		for _, value := range []string{"1", "7", `"secret"`} {
			t.Run(strings.Join([]string{map[bool]string{true: "query", false: "json"}[query], value}, "/"), func(t *testing.T) {
				t.Parallel()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"value":`+value+`}`))
				bind := BindJsonAndCheck
				if query {
					c.Request = httptest.NewRequest("GET", "/?value="+strings.Trim(value, `"`), nil)
					bind = BindQueryAndCheck
				}
				p := &checkedParam{}
				err := bind(c, p)
				switch value {
				case "1":
					require.NoError(t, err)
					require.Equal(t, 1, p.Value)
				case "7":
					require.EqualError(t, err, "custom validation")
				default:
					require.Error(t, err)
					require.NotContains(t, err.Error(), "secret")
					require.NotContains(t, err.Error(), "checkedParam")
				}
			})
		}
	}
}

func TestPlainJSON(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"value":1}`))
	p := struct {
		Value int `json:"value"`
	}{}
	require.NoError(t, BindJsonAndCheck(c, &p))
	require.Equal(t, 1, p.Value)
}

func TestShopAddValid(t *testing.T) {
	for _, name := range []string{"中心店", "", " \t\n"} {
		p := &ShopAdd{Name: name}
		if name == "中心店" {
			require.NoError(t, p.Valid())
		} else {
			require.Error(t, p.Valid())
		}
	}
}
