package log

import "go.uber.org/zap"

func InitializeMock() {
	Logger = zap.NewNop()
}
