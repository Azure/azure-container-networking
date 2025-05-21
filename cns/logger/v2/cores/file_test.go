package logger

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestFileConfig_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		have    []byte
		want    *FileConfig
		wantErr bool
	}{
		{
			name: "valid",
			have: []byte(`{"filepath":"test.log","level":"debug","maxBackups":5,"maxSize":10}`),
			want: &FileConfig{
				Filepath:   "test.log",
				Level:      "debug",
				level:      zapcore.DebugLevel,
				MaxBackups: 5,
				MaxSize:    10,
			},
		},
		{
			name:    "invalid level",
			have:    []byte(`{"filepath":"test.log","level":"invalid","maxBackups":5,"maxSize":10}`),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &FileConfig{}
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
