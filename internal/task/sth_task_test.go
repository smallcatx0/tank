package task

import (
	"encoding/json"
	"fmt"
	"gtank/models/dao"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var dbIns *gorm.DB

var (
	// 在单元测试文件中 初始化
	dbDsn string
)

func InitDb() {
	var err error
	dbIns, err = dao.GetTmpMysql(
		dbDsn,
	)
	if err != nil {
		log.Fatal(err.Error())
	}
}

func Test_GetTasks(t *testing.T) {
	InitDb()
	task := BsSthTask{}
	list, err := task.GetTasks(dbIns, 5)
	assert.NoError(t, err)
	PrintJson(list)
}

func PrintJson(v interface{}) {
	con, _ := json.MarshalIndent(v, "", "  ")
	fmt.Print(string(con) + "\n")
}
