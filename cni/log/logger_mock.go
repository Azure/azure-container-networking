package log

import "go.uber.org/zap"

func InitializeMock() {
	CNILogger.With(zap.String("component", "cni"))
}
