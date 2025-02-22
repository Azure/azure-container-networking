package logger

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-container-networking/internal/time"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestAIConfigUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		have    []byte
		want    *AppInsightsConfig
		wantErr bool
	}{
		{
			name: "valid",
			have: []byte(`{"grace_period":"30s","level":"panic","max_batch_interval":"30s","max_batch_size":32000}`),
			want: &AppInsightsConfig{
				GracePeriod:      time.Duration{Duration: 30 * time.Second},
				Level:            "panic",
				level:            zapcore.PanicLevel,
				MaxBatchInterval: time.Duration{Duration: 30 * time.Second},
				MaxBatchSize:     32000,
			},
		},
		{
			name:    "invalid level",
			have:    []byte(`{"level":"invalid"}`),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AppInsightsConfig{}
			err := json.Unmarshal(tt.have, c)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, c)
		})
	}
}
