package logger

import (
	"fmt"
	"maps"
	"os"
	"sync"

	ai "github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/log"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// wait time for closing AI telemetry session.
const waitTimeInSecs = 10

type logger struct {
	logger    *log.Logger
	zapLogger *zap.Logger
	th        ai.TelemetryHandle

	disableTraceLogging  bool
	disableMetricLogging bool
	disableEventLogging  bool

	m        sync.RWMutex
	metadata map[string]string
}

// Deprecated: The v1 logger is deprecated. Migrate to zap using the cns/logger/v2 package.
func New(fileName string, logLevel, logTarget int, logDir string) (loggershim, error) {
	l, err := log.NewLoggerE(fileName, logLevel, logTarget, logDir)
	if err != nil {
		return nil, errors.Wrap(err, "could not get new logger")
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	platformCore, err := getPlatformCores(zapcore.DebugLevel, jsonEncoder)
	if err != nil {
		l.Errorf("Failed to get zap Platform cores: %v", err)
	}
	zapLogger := zap.New(platformCore, zap.AddCaller()).With(zap.Int("pid", os.Getpid()))

	return &logger{
		logger:    l,
		zapLogger: zapLogger,
		metadata:  map[string]string{},
	}, nil
}

func (c *logger) InitAI(aiConfig ai.AIConfig, disableTraceLogging, disableMetricLogging, disableEventLogging bool) {
	c.InitAIWithIKey(aiConfig, aiMetadata, disableTraceLogging, disableMetricLogging, disableEventLogging)
}

func (c *logger) InitAIWithIKey(aiConfig ai.AIConfig, instrumentationKey string, disableTraceLogging, disableMetricLogging, disableEventLogging bool) {
	th, err := ai.NewAITelemetry("", instrumentationKey, aiConfig)
	if err != nil {
		c.logger.Errorf("Error initializing AI Telemetry:%v", err)
		return
	}
	c.th = th
	c.logger.Printf("AI Telemetry Handle created")
	c.disableMetricLogging = disableMetricLogging
	c.disableTraceLogging = disableTraceLogging
	c.disableEventLogging = disableEventLogging
}

func (c *logger) Close() {
	c.logger.Close()
	if c.th != nil {
		c.th.Close(waitTimeInSecs)
	}
}

func (c *logger) SetContextDetails(orchestrator, nodeID string) {
	c.logger.Logf("SetContext details called with: %v orchestrator nodeID %v", orchestrator, nodeID)
	c.m.Lock()
	c.metadata[orchestratorTypeKey] = orchestrator
	c.metadata[nodeIDKey] = nodeID
	c.m.Unlock()
}

func (c *logger) SetAPIServer(apiserver string) {
	c.m.Lock()
	c.metadata[apiServerKey] = apiserver
	c.m.Unlock()
}

func (c *logger) Printf(format string, args ...any) {
	c.logger.Logf(format, args...)
	c.zapLogger.Info(fmt.Sprintf(format, args...))
	if c.th == nil || c.disableTraceLogging {
		return
	}
	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg, ai.InfoLevel)
}

func (c *logger) Debugf(format string, args ...any) {
	c.logger.Debugf(format, args...)
	c.zapLogger.Debug(fmt.Sprintf(format, args...))
	if c.th == nil || c.disableTraceLogging {
		return
	}
	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg, ai.DebugLevel)
}

func (c *logger) Warnf(format string, args ...any) {
	c.logger.Warnf(format, args...)
	c.zapLogger.Warn(fmt.Sprintf(format, args...))
	if c.th == nil || c.disableTraceLogging {
		return
	}
	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg, ai.WarnLevel)
}

func (c *logger) Errorf(format string, args ...any) {
	c.logger.Errorf(format, args...)
	c.zapLogger.Error(fmt.Sprintf(format, args...))
	if c.th == nil || c.disableTraceLogging {
		return
	}
	msg := fmt.Sprintf(format, args...)
	c.sendTraceInternal(msg, ai.ErrorLevel)
}

func (c *logger) Request(tag string, request any, err error) {
	c.logger.Request(tag, request, err)
	if c.th == nil || c.disableTraceLogging {
		return
	}
	var msg string
	lvl := ai.InfoLevel
	if err == nil {
		msg = fmt.Sprintf("[%s] Received %T %+v.", tag, request, request)
	} else {
		msg = fmt.Sprintf("[%s] Failed to decode %T %+v %s.", tag, request, request, err.Error())
		lvl = ai.ErrorLevel
	}
	c.sendTraceInternal(msg, lvl)
}

func (c *logger) Response(tag string, response any, returnCode types.ResponseCode, err error) {
	c.logger.Response(tag, response, int(returnCode), returnCode.String(), err)
	if c.th == nil || c.disableTraceLogging {
		return
	}
	var msg string
	lvl := ai.InfoLevel
	switch {
	case err == nil && returnCode == 0:
		msg = fmt.Sprintf("[%s] Sent %T %+v.", tag, response, response)
	case err != nil:
		msg = fmt.Sprintf("[%s] Code:%s, %+v %s.", tag, returnCode.String(), response, err.Error())
		lvl = ai.ErrorLevel
	default:
		msg = fmt.Sprintf("[%s] Code:%s, %+v.", tag, returnCode.String(), response)
	}
	c.sendTraceInternal(msg, lvl)
}

func (c *logger) ResponseEx(tag string, request, response any, returnCode types.ResponseCode, err error) {
	c.logger.ResponseEx(tag, request, response, int(returnCode), returnCode.String(), err)
	if c.th == nil || c.disableTraceLogging {
		return
	}
	var msg string
	lvl := ai.InfoLevel
	switch {
	case err == nil && returnCode == 0:
		msg = fmt.Sprintf("[%s] Sent %T %+v %T %+v.", tag, request, request, response, response)
	case err != nil:
		msg = fmt.Sprintf("[%s] Code:%s, %+v, %+v, %s.", tag, returnCode.String(), request, response, err.Error())
		lvl = ai.ErrorLevel
	default:
		msg = fmt.Sprintf("[%s] Code:%s, %+v, %+v.", tag, returnCode.String(), request, response)
	}
	c.sendTraceInternal(msg, lvl)
}

func (c *logger) sendTraceInternal(msg string, lvl ai.Level) {
	report := ai.Report{
		Message:          msg,
		Level:            lvl,
		Context:          c.metadata[nodeIDKey],
		CustomDimensions: map[string]string{"Level": lvl.String()},
	}
	c.m.RLock()
	maps.Copy(report.CustomDimensions, c.metadata)
	c.m.RUnlock()
	c.th.TrackLog(report)
}

func (c *logger) LogEvent(event ai.Event) {
	if c.th == nil || c.disableEventLogging {
		return
	}
	c.m.RLock()
	maps.Copy(event.Properties, c.metadata)
	c.m.RUnlock()
	c.th.TrackEvent(event)
}

func (c *logger) SendMetric(metric ai.Metric) {
	if c.th == nil || c.disableMetricLogging {
		return
	}
	c.m.RLock()
	maps.Copy(metric.CustomDimensions, c.metadata)
	c.m.RUnlock()
	c.th.TrackMetric(metric)
}
