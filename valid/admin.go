package valid

type UserListParam struct {
	Id       int    `form:"id"`
	User     string `form:"user"`
	Nickname string `form:"nickname"`
	Truename string `form:"truename"`
	Phone    string `form:"phone"`
	Email    string `form:"email"`
	Status   string `form:"status"`
}
