package telemetry

import (
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
)

const (
	telemetryNumberRetries          = 5
	telemetryWaitTimeInMilliseconds = 200
)

type Client struct {
	cniReportSettings *CNIReport
	tb                *TelemetryBuffer
	logger            *zap.Logger
	lock              sync.Mutex
}

// package level variable for application insights telemetry
var AIClient = NewClient()

func NewClient() *Client {
	return &Client{
		cniReportSettings: &CNIReport{},
	}
}

// Settings gets a pointer to the cni report struct, used to modify individual fields
func (c *Client) Settings() *CNIReport {
	return c.cniReportSettings
}

// SetSettings REPLACES the pointer to the cni report struct and should only be used on startup
func (c *Client) SetSettings(settings *CNIReport) {
	c.cniReportSettings = settings
}

func (c *Client) IsConnected() bool {
	return c.tb != nil && c.tb.Connected
}

func (c *Client) ConnectTelemetry(logger *zap.Logger) {
	c.tb = NewTelemetryBuffer(logger)
	c.tb.ConnectToTelemetry()
	c.logger = logger
}

func (c *Client) StartAndConnectTelemetry(logger *zap.Logger) {
	c.tb = NewTelemetryBuffer(logger)
	c.tb.ConnectToTelemetryService(telemetryNumberRetries, telemetryWaitTimeInMilliseconds)
	c.logger = logger
}

func (c *Client) DisconnectTelemetry() {
	if c.tb == nil {
		return
	}
	c.tb.Close()
}

func (c *Client) sendEvent(msg string) {
	if c.tb == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	eventMsg := fmt.Sprintf("[%d] %s", os.Getpid(), msg)
	c.cniReportSettings.EventMessage = eventMsg
	SendCNIEvent(c.tb, c.cniReportSettings)
}

func (c *Client) sendLog(msg string) {
	if c.logger == nil {
		return
	}
	c.logger.Info("Telemetry Event", zap.String("message", msg))
}

func (c *Client) SendEvent(msg string) {
	c.sendEvent(msg)
}

func (c *Client) SendError(err error) {
	if err == nil {
		return
	}
	// when the cni report reaches the telemetry service, the ai log message
	// is set to either the cni report's event message or error message,
	// whichever is not empty, so we can always just set the event message
	c.sendEvent(err.Error())
}

func (c *Client) SendMetric(cniMetric *AIMetric) {
	if c.tb == nil || cniMetric == nil {
		return
	}
	err := SendCNIMetric(cniMetric, c.tb)
	if err != nil {
		c.sendLog("Couldn't send metric: " + err.Error())
	}
}
