package logger

import (
	"encoding/json"
	"testing"

	ai "github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/log"
	"github.com/stretchr/testify/require"
)

type telemetryRecorder struct {
	reports []ai.Report
}

func (r *telemetryRecorder) TrackLog(report ai.Report) {
	r.reports = append(r.reports, report)
}

func (*telemetryRecorder) TrackMetric(ai.Metric) {}

func (*telemetryRecorder) TrackEvent(ai.Event) {}

func (*telemetryRecorder) Close(int) {}

func (*telemetryRecorder) Flush() {}

type rawMessagePayload struct {
	Data json.RawMessage
}

func TestLogger_TraceLogsUseJSON(t *testing.T) {
	request := rawMessagePayload{Data: json.RawMessage(`{"name":"request"}`)}
	response := rawMessagePayload{Data: json.RawMessage(`{"name":"response"}`)}

	tests := []struct {
		name string
		log  func(*logger)
		want string
	}{
		{
			name: "request",
			log: func(c *logger) {
				c.Request("request", request, nil)
			},
			want: `{"Data":{"name":"request"}}`,
		},
		{
			name: "response",
			log: func(c *logger) {
				c.Response("response", response, types.ResponseCode(0), nil)
			},
			want: `{"Data":{"name":"response"}}`,
		},
		{
			name: "response with request",
			log: func(c *logger) {
				c.ResponseEx("response-ex", request, response, types.ResponseCode(0), nil)
			},
			want: `{"Data":{"name":"request"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetry := &telemetryRecorder{}
			baseLogger, err := log.NewLoggerE("trace-test", log.LevelAlert, log.TargetStderr, "")
			require.NoError(t, err)
			t.Cleanup(baseLogger.Close)

			tt.log(&logger{
				logger:   baseLogger,
				th:       telemetry,
				metadata: map[string]string{},
			})

			require.Len(t, telemetry.reports, 1)
			require.Contains(t, telemetry.reports[0].Message, tt.want)
			require.NotContains(t, telemetry.reports[0].Message, "[123")
		})
	}
}
