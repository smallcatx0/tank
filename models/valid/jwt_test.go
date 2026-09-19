package valid_test

import (
	"fmt"
	"gtank/models/valid"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_jwtGenerate(t *testing.T) {
	j := &valid.JWTData{
		Uid:   12,
		Phone: "18681636749",
	}
	s, err := j.Generate()
	assert.NoError(t, err)
	fmt.Println(s)
}

func Test_JWTParse(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2NDUzNjM4NzksInVpZCI6IjEyIiwicGhvbmUiOiIxODY4MTYzNjc0OSJ9.eqYNdzfeFqwlPx5Z34hQ9yXjSCcCw3MKLEkoNQl6x6k"
	res, err := valid.JWTParse(token)
	assert.NoError(t, err)
	fmt.Println(res)
}
