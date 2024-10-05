package slscal

import (
	"sort"
	"strconv"
	"testing"
	"time"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	"github.com/aliyun/aliyun-log-go-sdk/producer"
	"google.golang.org/protobuf/proto"
)

func buildLogItem(metricName string, labels map[string]string, value float64) *sls.Log {

	now := uint32(time.Now().Unix())
	log := &sls.Log{Time: proto.Uint32(now)}

	var contents []*sls.LogContent
	contents = append(contents, &sls.LogContent{
		Key:   proto.String("__time_nano__"),
		Value: proto.String(strconv.FormatInt(int64(now), 10)),
	})
	contents = append(contents, &sls.LogContent{
		Key:   proto.String("__name__"),
		Value: proto.String(metricName),
	})
	contents = append(contents, &sls.LogContent{
		Key:   proto.String("__value__"),
		Value: proto.String(strconv.FormatFloat(value, 'f', 6, 64)),
	})

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	labelsStr := ""
	for i, k := range keys {
		labelsStr += k
		labelsStr += "#$#"
		labelsStr += labels[k]
		if i < len(keys)-1 {
			labelsStr += "|"
		}
	}

	contents = append(contents, &sls.LogContent{Key: proto.String("__labels__"), Value: proto.String(labelsStr)})
	log.Contents = contents
	return log

}

var slsConfig *producer.ProducerConfig

func TestTry(t *testing.T) {
	project := "try-alisls"
	logstore := "test_scan"

	producerInstance := producer.InitProducer(slsConfig)
	producerInstance.Start()
	logs := make([]*sls.Log, 0, 10)
	for i := 0; i < 10; i++ {
		logs = append(logs, buildLogItem("test_metric", map[string]string{"key1": strconv.Itoa(i), "key2": "vvv"}, float64(i)))
	}
	producerInstance.SendLogList(project, logstore, "", "local-test", logs)

	time.Sleep(time.Second * 30)
}
