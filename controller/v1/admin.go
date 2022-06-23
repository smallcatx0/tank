package v1

import (
	"gtank/middleware/resp"
	"gtank/models/dao"
	"gtank/models/dao/mdb"
	"gtank/valid"

	"github.com/gin-gonic/gin"
)

type UserAdmin struct{}

func (UserAdmin) List(c *gin.Context) {
	p := valid.UserListParam{}
	err := valid.BindQueryAndCheck(c, &p)
	if err != nil {
		resp.Fail(c, err)
		return
	}

	q := dao.MDB.Model(&mdb.User{}).Omit("pass")
	if p.Id != 0 {
		q = q.Where("id=?", p.Id)
	}
	if p.User != "" {
		q = q.Where("user=?", p.User)
	}
	if p.Nickname != "" {
		q = q.Where("nickname=?", p.Nickname)
	}
	if p.Truename != "" {
		q = q.Where("truename=?", p.Truename)
	}
	if p.Email != "" {
		q = q.Where("email=?", p.Email)
	}
	if p.Status != "" {
		q = q.Where("status=?", p.Status)
	}
	pg := resp.NewPage(c)
	q, err = pg.Paginate(q)
	if err != nil {
		resp.Fail(c, err)
		return
	}
	if pg.Total == 0 {
		resp.Paginate(c, *pg, nil)
		return
	}
	users := make([]mdb.User, 0, pg.Limit)
	err = q.Find(&users).Error
	if err != nil {
		resp.Fail(c, err)
		return
	}
	resp.Paginate(c, *pg, users)
}
