package log

import (
	"go.uber.org/zap"
)

func InitializeMock() {
	InitZapLogCNI("azure-vnet", "").With(zap.String("component", "cni"))
}
