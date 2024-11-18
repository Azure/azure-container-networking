package telemetryclient

import (
	"fmt"
	"os"
	"sync"

	"github.com/Azure/azure-container-networking/telemetry"
	"go.uber.org/zap"
)

const (
	telemetryNumberRetries          = 5
	telemetryWaitTimeInMilliseconds = 200
)

type TelemetryClient struct {
	cniReportSettings *telemetry.CNIReport
	tb                *telemetry.TelemetryBuffer
	logger            *zap.Logger
	lock              sync.Mutex
}

type TelemetryInterface interface {
	// Settings gets a pointer to the cni report struct, used to modify individual fields
	Settings() *telemetry.CNIReport
	// SetSettings REPLACES the pointer to the cni report struct and should only be used on startup
	SetSettings(settings *telemetry.CNIReport)
	IsConnected() bool
	ConnectTelemetry(logger *zap.Logger)
	StartAndConnectTelemetry(logger *zap.Logger)
	DisconnectTelemetry()
	SendEvent(msg string)
	SendError(err error)
	SendMetric(cniMetric *telemetry.AIMetric)
}

var Telemetry TelemetryInterface = NewTelemetryClient()

func NewTelemetryClient() *TelemetryClient {
	return &TelemetryClient{
		cniReportSettings: &telemetry.CNIReport{},
	}
}

func (c *TelemetryClient) Settings() *telemetry.CNIReport {
	return c.cniReportSettings
}

func (c *TelemetryClient) SetSettings(settings *telemetry.CNIReport) {
	c.cniReportSettings = settings
}

func (c *TelemetryClient) IsConnected() bool {
	return c.tb != nil && c.tb.Connected
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

func (c *TelemetryClient) sendEvent(msg string) {
	if c.tb == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	eventMsg := fmt.Sprintf("[%d] %s", os.Getpid(), msg)
	c.cniReportSettings.EventMessage = eventMsg
	telemetry.SendCNIEvent(c.tb, c.cniReportSettings)
}

func (c *TelemetryClient) sendLog(msg string) {
	if c.logger == nil {
		return
	}
	c.logger.Info("Telemetry Event", zap.String("message", msg))
}

func (c *TelemetryClient) SendEvent(msg string) {
	c.sendLog(msg)
	c.sendEvent(msg)
}

func (c *TelemetryClient) SendError(err error) {
	if err == nil {
		return
	}
	// when the cni report reaches the telemetry service, the ai log message
	// is set to either the cni report's event message or error message,
	// whichever is not empty, so we can always just set the event message
	c.sendEvent(err.Error())
}

func (c *TelemetryClient) SendMetric(cniMetric *telemetry.AIMetric) {
	if c.tb == nil || cniMetric == nil {
		return
	}
	err := telemetry.SendCNIMetric(cniMetric, c.tb)
	if err != nil {
		c.logger.Error("Couldn't send metric", zap.Error(err))
	}
}
