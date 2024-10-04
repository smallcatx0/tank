package slscal

import "go.uber.org/zap"

type SLSUpper struct {
	Endpoint string
	Project  string
	Logstore string
	Ak       string
	Sk       string
	Log      *zap.Logger
}
