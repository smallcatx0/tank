package valid

import "fmt"

type UserLogin struct {
	UserName string `json:"user_name" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (u *UserLogin) Valid() error {
	if u.UserName == "admin" {
		return fmt.Errorf("not allow")
	}
	return nil
}
