// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var telemetryTests = []struct {
	name    string
	Data    interface{}
	wantErr bool
}{
	{
		name: "aimetric type",
		Data: &AIMetric{
			Metric: aitelemetry.Metric{
				Name:             "test",
				Value:            float64(1.0),
				CustomDimensions: make(map[string]string),
			},
		},
		wantErr: false,
	},
	{
		name: "cnireport type",
		Data: &CNIReport{
			Name:              "test-cnireport",
			Version:           "11",
			OperationDuration: 10,
			Context:           "test-context",
			SubContext:        "test-subcontext",
		},
		wantErr: false,
	},
	{
		name:    "nil type",
		Data:    nil,
		wantErr: true,
	},
	{
		name:    "unexpected type",
		Data:    1,
		wantErr: true,
	},
}

func TestMain(m *testing.M) {
	telemetryLog := log.CNILogger.With(zap.String("component", "cni-telemetry"))
	tb := NewTelemetryBuffer(telemetryLog)
	_ = tb.Cleanup(FdName)
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestReportToBytes(t *testing.T) {
	reportManager := &ReportManager{}
	for _, tt := range telemetryTests {
		tt := tt
		reportManager.Report = tt.Data
		t.Run(tt.name, func(t *testing.T) {
			_, err := reportManager.ReportToBytes()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSendReport(t *testing.T) {
	tb, closeTBServer := createTBServer(t)
	defer closeTBServer()

	err := tb.Connect()
	require.NoError(t, err)

	reportManager := &ReportManager{}
	for _, tt := range telemetryTests {
		tt := tt
		reportManager.Report = tt.Data
		t.Run(tt.name, func(t *testing.T) {
			err := reportManager.SendReport(tb)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSendCNIMetric(t *testing.T) {
	tb, closeTBServer := createTBServer(t)
	defer closeTBServer()

	err := tb.Connect()
	require.NoError(t, err)

	tests := []struct {
		name    string
		metric  *AIMetric
		wantErr bool
	}{
		{
			name: "aimetric",
			metric: &AIMetric{
				Metric: aitelemetry.Metric{
					Name:             "test-metric",
					Value:            float64(1.0),
					CustomDimensions: make(map[string]string),
				},
			},
			wantErr: false,
		},
		{
			name:    "nil",
			metric:  nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := SendCNIMetric(tt.metric, tb)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGetOSDetails(t *testing.T) {
	expectedErrMsg := ""
	cniReport := &CNIReport{}
	cniReport.GetSystemDetails()
	require.Equal(t, expectedErrMsg, cniReport.ErrorMessage)
}

func TestGetSystemDetails(t *testing.T) {
	expectedErrMsg := ""
	cniReport := &CNIReport{}
	cniReport.GetSystemDetails()
	require.Equal(t, expectedErrMsg, cniReport.ErrorMessage)
}
