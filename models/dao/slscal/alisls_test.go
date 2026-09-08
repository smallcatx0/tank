package slscal

import (
	"testing"
	"time"
)

func Test_UpSLSData(t *testing.T) {
	var (
		data = []map[string]string{
			{"dm": "www.abc.com", "dmid": "123123", "succ_cnt": "35"},
			{"dm": "www.abc.com", "dmid": "123123", "succ_cnt": "35"},
			{"dm": "www.abc.com", "dmid": "123123", "succ_cnt": "35"},
			{"dm": "www.abc.com", "dmid": "123123", "succ_cnt": "35"},
			{"dm": "www.abc.com", "dmid": "123123", "succ_cnt": "35"},
		}
	)
	project := "try-alisls"
	logstore := "test_scan"
	up := NewSLSUpper(
		slsConfig.Endpoint,
		project, logstore,
		slsConfig.AccessKeyID,
		slsConfig.AccessKeySecret,
	)
	log := Map2SLSItems(data)
	up.SendLogs("unit_test", log)

	time.Sleep(time.Second * 20)
}
