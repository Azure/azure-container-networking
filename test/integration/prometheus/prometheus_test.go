package prometheus

import (
	"testing"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestGetMetricValue(t *testing.T) {
	metrics := map[string]*io_prometheus_client.MetricFamily{
		"test_metric_0": {
			Metric: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{
						{Name: ptr("scenario"), Value: ptr("local")},
						{Name: ptr("sku"), Value: ptr("large")},
					},
					Counter: &io_prometheus_client.Counter{Value: ptrFloat(30.0)},
				},
			},
		},
		"test_metric_1": {
			Metric: []*io_prometheus_client.Metric{
				{
					Label: []*io_prometheus_client.LabelPair{
						{Name: ptr("instance"), Value: ptr("localhost")},
						{Name: ptr("job"), Value: ptr("test")},
					},
					Counter: &io_prometheus_client.Counter{Value: ptrFloat(42.5)},
				},
				{
					Label: []*io_prometheus_client.LabelPair{
						{Name: ptr("instance"), Value: ptr("remotehost")},
						{Name: ptr("job"), Value: ptr("test")},
					},
					Counter: &io_prometheus_client.Counter{Value: ptrFloat(55.0)},
				},
			},
		},
	}

	tests := []struct {
		name        string
		metricName  string
		target      map[string]string
		expectedVal float64
		expectErr   bool
	}{
		{
			name:       "Match metric",
			metricName: "test_metric_0",
			target: map[string]string{
				"sku":      "large",
				"scenario": "local",
			},
			expectedVal: 30,
			expectErr:   false,
		},
		{
			name:       "Match first metric",
			metricName: "test_metric_1",
			target: map[string]string{
				"instance": "localhost",
				"job":      "test",
			},
			expectedVal: 42.5,
			expectErr:   false,
		},
		{
			name:       "Match second metric",
			metricName: "test_metric_1",
			target: map[string]string{
				"instance": "remotehost",
				"job":      "test",
			},
			expectedVal: 55.0,
			expectErr:   false,
		},
		{
			name:       "Metric not found",
			metricName: "non_existent_metric",
			target:     map[string]string{"instance": "localhost"},
			expectErr:  true,
		},
		{
			name:       "No matching labels",
			metricName: "test_metric_1",
			target: map[string]string{
				"instance": "missing_host",
			},
			expectErr: true,
		},
		{
			name:       "No exact match",
			metricName: "test_metric_1",
			target: map[string]string{
				"instance": "localhost",
				"job":      "foo",
			},
			expectErr: true,
		},
		{
			name:       "Different number of labels",
			metricName: "test_metric_1",
			target: map[string]string{
				"instance": "localhost",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric, err := SelectMetric(metrics, tt.metricName, tt.target)
			val := metric.GetCounter().GetValue()

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.InDelta(t, tt.expectedVal, val, 0.01)
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}

func ptrFloat(f float64) *float64 {
	return &f
}
