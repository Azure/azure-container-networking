package log

import (
	"go.uber.org/zap"
)

func InitializeMock() {
	InitZapLogCNI(LoggerVnetCfg).With(zap.String("component", "cni"))
}
