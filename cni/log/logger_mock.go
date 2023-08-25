package log

import (
	"github.com/Azure/azure-container-networking/zaplog"
	"go.uber.org/zap"
)

func InitializeMock() {
	zaplog.InitLog(LoggerVnetCfg).With(zap.String("component", "cni"))
}
