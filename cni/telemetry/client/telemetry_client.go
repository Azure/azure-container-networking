package telemetryclient

import (
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
)

const (
	telemetryNumberRetries          = 5
	telemetryWaitTimeInMilliseconds = 200
)

type TelemetryClient struct {
	CNIReportSettings *telemetry.CNIReport
	tb                *telemetry.TelemetryBuffer
	logger            *zap.Logger
}

var Telemetry = NewTelemetryClient(&telemetry.CNIReport{})

func NewTelemetryClient(report *telemetry.CNIReport) *TelemetryClient {
	return &TelemetryClient{
		CNIReportSettings: report,
	}
}

func (c *TelemetryClient) ConnectTelemetry(logger *zap.Logger) {
	c.tb = telemetry.NewTelemetryBuffer(logger)
	c.tb.ConnectToTelemetry()
	c.logger = logger
}

func (c *TelemetryClient) StartAndConnectTelemetry(logger *zap.Logger) {
	c.tb = telemetry.NewTelemetryBuffer(logger)
	c.tb.ConnectToTelemetryService(telemetryNumberRetries, telemetryWaitTimeInMilliseconds)
	c.logger = logger
}

func (c *TelemetryClient) DisconnectTelemetry() {
	if c.tb == nil {
		return
	}
	c.tb.Close()
}
func (c *TelemetryClient) sendTelemetry(msg string) {
	if c.tb == nil {
		return
	}
	c.CNIReportSettings.EventMessage = msg
	eventMsg := fmt.Sprintf("[%d] %s", os.Getpid(), msg)
	c.CNIReportSettings.EventMessage = eventMsg
	telemetry.SendCNIEvent(c.tb, c.CNIReportSettings)
}
func (c *TelemetryClient) sendLog(msg string) {
	if c.logger == nil {
		return
	}
	c.logger.Info("Telemetry Event", zap.String("message", msg))
}
func (c *TelemetryClient) SendEvent(msg string) {
	c.sendLog(msg)
	c.sendTelemetry(msg)
}
