package logger

import (
	"fmt"
	"sync"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/log"
	"github.com/pkg/errors"
)

type CNSLogger struct {
	logger               *log.Logger
	th                   aitelemetry.TelemetryHandle
	DisableTraceLogging  bool
	DisableMetricLogging bool
	DisableEventLogging  bool

	m            sync.RWMutex
	Orchestrator string
	NodeID       string
}

func NewCNSLogger(fileName string, logLevel, logTarget int, logDir string) (*CNSLogger, error) {
	l, err := log.NewLoggerE(fileName, logLevel, logTarget, logDir)
	if err != nil {
		return nil, errors.Wrap(err, "could not get new logger")
	}

	return &CNSLogger{logger: l}, nil
}

func (c *CNSLogger) InitAI(aiConfig aitelemetry.AIConfig, disableTraceLogging, disableMetricLogging, disableEventLogging bool) {
	th, err := aitelemetry.NewAITelemetry("", aiMetadata, aiConfig)
	if err != nil {
		c.logger.Errorf("Error initializing AI Telemetry:%v", err)
		return
	}

	c.th = th
	c.logger.Printf("AI Telemetry Handle created")
	c.DisableMetricLogging = disableMetricLogging
	c.DisableTraceLogging = disableTraceLogging
	c.DisableEventLogging = disableEventLogging
}

// wait time for closing AI telemetry session.
const waitTimeInSecs = 10

func (c *CNSLogger) Close() {
	c.logger.Close()
	if c.th != nil {
		c.th.Close(waitTimeInSecs)
	}
}

func (c *CNSLogger) SetContextDetails(orchestrator, nodeID string) {
	c.logger.Logf("SetContext details called with: %v orchestrator nodeID %v", orchestrator, nodeID)
	c.m.Lock()
	c.Orchestrator = orchestrator
	c.NodeID = nodeID
	c.m.Unlock()
}

func (c *CNSLogger) Printf(format string, args ...any) {
	c.logger.Logf(format, args...)

	if c.th == nil || c.DisableTraceLogging {
		return
	}

	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg)
}

func (c *CNSLogger) Debugf(format string, args ...any) {
	c.logger.Debugf(format, args...)

	if c.th == nil || c.DisableTraceLogging {
		return
	}

	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg)
}

func (c *CNSLogger) Warnf(format string, args ...any) {
	c.logger.Warnf(format, args...)

	if c.th == nil || c.DisableTraceLogging {
		return
	}

	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg)
}

func (c *CNSLogger) Errorf(format string, args ...any) {
	c.logger.Errorf(format, args...)

	if c.th == nil || c.DisableTraceLogging {
		return
	}

	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg)
}

func (c *CNSLogger) Request(tag string, request any, err error) {
	c.logger.Request(tag, request, err)

	if c.th == nil || c.DisableTraceLogging {
		return
	}

	var msg string
	if err == nil {
		msg = fmt.Sprintf("[%s] Received %T %+v.", tag, request, request)
	} else {
		msg = fmt.Sprintf("[%s] Failed to decode %T %+v %s.", tag, request, request, err.Error())
	}

	c.sendTraceInternal(msg)
}

func (c *CNSLogger) Response(tag string, response any, returnCode types.ResponseCode, err error) {
	c.logger.Response(tag, response, int(returnCode), returnCode.String(), err)

	if c.th == nil || c.DisableTraceLogging {
		return
	}

	var msg string
	switch {
	case err == nil && returnCode == 0:
		msg = fmt.Sprintf("[%s] Sent %T %+v.", tag, response, response)
	case err != nil:
		msg = fmt.Sprintf("[%s] Code:%s, %+v %s.", tag, returnCode.String(), response, err.Error())
	default:
		msg = fmt.Sprintf("[%s] Code:%s, %+v.", tag, returnCode.String(), response)
	}

	c.sendTraceInternal(msg)
}

func (c *CNSLogger) ResponseEx(tag string, request, response any, returnCode types.ResponseCode, err error) {
	c.logger.ResponseEx(tag, request, response, int(returnCode), returnCode.String(), err)

	if c.th == nil || c.DisableTraceLogging {
		return
	}

	var msg string
	switch {
	case err == nil && returnCode == 0:
		msg = fmt.Sprintf("[%s] Sent %T %+v %T %+v.", tag, request, request, response, response)
	case err != nil:
		msg = fmt.Sprintf("[%s] Code:%s, %+v, %+v, %s.", tag, returnCode.String(), request, response, err.Error())
	default:
		msg = fmt.Sprintf("[%s] Code:%s, %+v, %+v.", tag, returnCode.String(), request, response)
	}

	c.sendTraceInternal(msg)
}

func (c *CNSLogger) getOrchestratorAndNodeID() (orch, nodeID string) {
	c.m.RLock()
	orch, nodeID = c.Orchestrator, c.NodeID
	c.m.RUnlock()
	return
}

func (c *CNSLogger) sendTraceInternal(msg string) {
	orch, nodeID := c.getOrchestratorAndNodeID()

	report := aitelemetry.Report{
		Message: msg,
		Context: nodeID,
		CustomDimensions: map[string]string{
			OrchestratorTypeStr: orch,
			NodeIDStr:           nodeID,
		},
	}

	c.th.TrackLog(report)
}

func (c *CNSLogger) LogEvent(event aitelemetry.Event) {
	if c.th == nil || c.DisableEventLogging {
		return
	}

	orch, nodeID := c.getOrchestratorAndNodeID()
	event.Properties[OrchestratorTypeStr] = orch
	event.Properties[NodeIDStr] = nodeID
	c.th.TrackEvent(event)
}

func (c *CNSLogger) SendMetric(metric aitelemetry.Metric) {
	if c.th == nil || c.DisableMetricLogging {
		return
	}

	orch, nodeID := c.getOrchestratorAndNodeID()
	metric.CustomDimensions[OrchestratorTypeStr] = orch
	metric.CustomDimensions[NodeIDStr] = nodeID
	c.th.TrackMetric(metric)
}
