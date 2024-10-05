package slscal

import (
	"os"
	"time"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	"github.com/aliyun/aliyun-log-go-sdk/producer"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var hostname string

func Hostname() string {
	if hostname != "" {
		return hostname
	}
	hostname, _ = os.Hostname()
	return hostname
}

type SLSUpper struct {
	Endpoint string
	Project  string
	Logstore string
	ak       string
	sk       string
	Log      *zap.Logger
	cfg      *producer.ProducerConfig
	Producer *producer.Producer
}

type upCallback struct {
	Zap *zap.Logger
	Cnt int
}

func (up *upCallback) Success(res *producer.Result) {
	up.Zap.Info("[SLSUpper] 上传sls数据成功", zap.Int("cnt", up.Cnt))
}
func (up *upCallback) Fail(res *producer.Result) {
	up.Zap.Error("[SLSUpper] 上传sls数据失败: err="+res.GetErrorCode()+","+res.GetErrorMessage(), zap.String("req_id", res.GetRequestId()), zap.Int("cnt", up.Cnt))
}

func NewSLSUpper(endpoint, project, logstore, ak, sk string) *SLSUpper {
	upper := &SLSUpper{
		Endpoint: endpoint,
		Project:  project,
		Logstore: logstore,
		ak:       ak,
		sk:       sk,
		Log:      zap.NewExample(),
	}
	upper.cfg = producer.GetDefaultProducerConfig()
	upper.cfg.Endpoint = endpoint
	upper.cfg.AccessKeyID = ak
	upper.cfg.AccessKeySecret = sk

	upper.Producer = producer.InitProducer(upper.cfg)

	upper.Producer.Start()
	return upper
}

func (s *SLSUpper) Close() {
	s.Producer.SafeClose()
}

func (s *SLSUpper) SendLogs(topic string, logs []*sls.Log) error {
	call := upCallback{
		Zap: s.Log,
		Cnt: len(logs),
	}
	err := s.Producer.SendLogListWithCallBack(s.Project, s.Logstore, topic, Hostname(), logs, &call)
	return err
}

func Map2SLSItem(data map[string]string) *sls.Log {
	now := uint32(time.Now().Unix())
	log := &sls.Log{Time: proto.Uint32(now)}
	for k, v := range data {
		log.Contents = append(log.Contents, &sls.LogContent{
			Key:   proto.String(k),
			Value: proto.String(v),
		})
	}
	return log
}

func Map2SLSItems(datas []map[string]string) []*sls.Log {
	ret := make([]*sls.Log, 0, len(datas))
	for _, one := range datas {
		ret = append(ret, Map2SLSItem(one))
	}
	return ret
}
