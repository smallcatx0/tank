package cronjob

import "go.uber.org/zap"

type CommandFunc struct {
	FuncName string
	Args     []string
}

func MakeCommandFunc(cfg CommandFunc, logger *zap.Logger) (func(), error) {

	return nil, nil
}
