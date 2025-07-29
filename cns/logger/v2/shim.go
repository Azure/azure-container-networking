package logger

import (
	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/types"
	"go.uber.org/zap"
)

// shim wraps the Zap logger to provide a compatible interface to the
// legacy CNS logger. This is temporary and exists to make migration
// feasible and optional.
type shim struct {
	z      *zap.Logger
	closer func()
}

func (s *shim) Close() {
	_ = s.z.Sync()
	s.closer()
}

func (s *shim) Printf(format string, a ...any) {
	s.z.Sugar().Infof(format, a...)
}

func (s *shim) Debugf(format string, a ...any) {
	s.z.Sugar().Debugf(format, a...)
}

func (s *shim) Warnf(format string, a ...any) {
	s.z.Sugar().Warnf(format, a...)
}

func (s *shim) Errorf(format string, a ...any) {
	s.z.Sugar().Errorf(format, a...)
}

func (s *shim) Request(msg string, data any, err error) {
	s.z.Sugar().Infow("Request", "message", msg, "data", data, "error", err)
}

func (s *shim) Response(msg string, data any, code types.ResponseCode, err error) {
	s.z.Sugar().Infow("Response", "message", msg, "data", data, "code", code, "error", err)
}

func (s *shim) ResponseEx(msg string, request, response any, code types.ResponseCode, err error) {
	s.z.Sugar().Infow("ResponseEx", "message", msg, "request", request, "response", response, "code", code, "error", err)
}

func (*shim) InitAI(aitelemetry.AIConfig, bool, bool, bool) {}

func (*shim) InitAIWithIKey(aitelemetry.AIConfig, string, bool, bool, bool) {}

func (s *shim) SetContextDetails(string, string) {}

func (s *shim) SetAPIServer(string) {}

func (s *shim) SendMetric(aitelemetry.Metric) {}

func (s *shim) LogEvent(aitelemetry.Event) {}

func AsV1(z *zap.Logger, closer func()) *shim { //nolint:revive // I want it to be annoying to use.
	return &shim{z: z, closer: closer}
}
