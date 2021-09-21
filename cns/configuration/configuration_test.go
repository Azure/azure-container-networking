package configuration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigFilePath(t *testing.T) {
	execpath, _ := common.GetExecutableDirectory()

	// env unset
	f, err := getConfigFilePath()
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(execpath, defaultConfigName), f)

	// env set
	os.Setenv(EnvCNSConfig, "test.cfg")
	f, err = getConfigFilePath()
	assert.NoError(t, err)
	assert.Equal(t, "test.cfg", f)
}

func TestReadConfigFromFile(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    *CNSConfig
		wantErr bool
	}{
		{
			name:    "not found",
			path:    "/dne",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "malformed json",
			path:    "testdata/bad.txt",
			want:    nil,
			wantErr: true,
		},
		{
			name: "full config",
			path: "testdata/good.json",
			want: &CNSConfig{
				ChannelMode:       "Direct",
				InitializeFromCNI: true,
				ManagedSettings: ManagedSettings{
					PrivateEndpoint:           "abc",
					InfrastructureNetworkID:   "abc",
					NodeID:                    "abc",
					NodeSyncIntervalInSeconds: 30,
				},
				MetricsBindAddress:          ":9091",
				SyncHostNCTimeoutMs:         5,
				SyncHostNCVersionIntervalMs: 5,
				TLSCertificatePath:          "/test",
				TLSEndpoint:                 "0.0.0.0",
				TLSPort:                     "10091",
				TLSSubjectName:              "subj",
				TelemetrySettings: TelemetrySettings{
					DebugMode:                    true,
					DisableAll:                   true,
					DisableEvent:                 true,
					DisableMetadataRefreshThread: true,
					DisableMetric:                true,
					DisableTrace:                 true,
					HeartBeatIntervalInMins:      30,
					RefreshIntervalInSecs:        15,
					SnapshotIntervalInMins:       60,
					TelemetryBatchIntervalInSecs: 15,
					TelemetryBatchSizeBytes:      16384,
				},
				UseHTTPS:     true,
				WireserverIP: "168.63.129.16",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := readConfigFromFile(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetTelemetrySettingDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   TelemetrySettings
		want TelemetrySettings
	}{
		{
			name: "set defaults",
			in:   TelemetrySettings{},
			want: TelemetrySettings{
				RefreshIntervalInSecs:        15,
				TelemetryBatchIntervalInSecs: 30,
				TelemetryBatchSizeBytes:      32768,
				HeartBeatIntervalInMins:      30,
				SnapshotIntervalInMins:       60,
			},
		},
		{
			name: "don't override set values",
			in: TelemetrySettings{
				RefreshIntervalInSecs:        3,
				TelemetryBatchIntervalInSecs: 4,
				TelemetryBatchSizeBytes:      5,
				HeartBeatIntervalInMins:      6,
				SnapshotIntervalInMins:       7,
			},
			want: TelemetrySettings{
				RefreshIntervalInSecs:        3,
				TelemetryBatchIntervalInSecs: 4,
				TelemetryBatchSizeBytes:      5,
				HeartBeatIntervalInMins:      6,
				SnapshotIntervalInMins:       7,
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			setTelemetrySettingDefaults(&tt.in)
			assert.Equal(t, tt.want, tt.in)
		})
	}
}

func Test_setManagedSettingDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   ManagedSettings
		want ManagedSettings
	}{
		{
			name: "set defaults",
			in:   ManagedSettings{},
			want: ManagedSettings{
				NodeSyncIntervalInSeconds: 30,
			},
		},
		{
			name: "don't override set values",
			in: ManagedSettings{
				NodeSyncIntervalInSeconds: 5,
			},
			want: ManagedSettings{
				NodeSyncIntervalInSeconds: 5,
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			setManagedSettingDefaults(&tt.in)
			assert.Equal(t, tt.want, tt.in)
		})
	}
}

func TestSetCNSConfigDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   CNSConfig
		want CNSConfig
	}{
		{
			name: "unset defaults",
			in:   CNSConfig{},
			want: CNSConfig{
				ChannelMode: "Direct",
				ManagedSettings: ManagedSettings{
					NodeSyncIntervalInSeconds: 30,
				},
				MetricsBindAddress:          ":9090",
				SyncHostNCTimeoutMs:         500,
				SyncHostNCVersionIntervalMs: 1000,
				TelemetrySettings: TelemetrySettings{
					TelemetryBatchSizeBytes:      32768,
					TelemetryBatchIntervalInSecs: 30,
					HeartBeatIntervalInMins:      30,
					RefreshIntervalInSecs:        15,
					SnapshotIntervalInMins:       60,
				},
			},
		},
		{
			name: "don't overwrite set values",
			in: CNSConfig{
				ChannelMode: "Other",
				ManagedSettings: ManagedSettings{
					NodeSyncIntervalInSeconds: 1,
				},
				MetricsBindAddress:          ":9091",
				SyncHostNCTimeoutMs:         5,
				SyncHostNCVersionIntervalMs: 1,
				TelemetrySettings: TelemetrySettings{
					TelemetryBatchSizeBytes:      3,
					TelemetryBatchIntervalInSecs: 3,
					HeartBeatIntervalInMins:      3,
					RefreshIntervalInSecs:        1,
					SnapshotIntervalInMins:       6,
				},
			},
			want: CNSConfig{
				ChannelMode: "Other",
				ManagedSettings: ManagedSettings{
					NodeSyncIntervalInSeconds: 1,
				},
				MetricsBindAddress:          ":9091",
				SyncHostNCTimeoutMs:         5,
				SyncHostNCVersionIntervalMs: 1,
				TelemetrySettings: TelemetrySettings{
					TelemetryBatchSizeBytes:      3,
					TelemetryBatchIntervalInSecs: 3,
					HeartBeatIntervalInMins:      3,
					RefreshIntervalInSecs:        1,
					SnapshotIntervalInMins:       6,
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			SetCNSConfigDefaults(&tt.in)
			assert.Equal(t, tt.want, tt.in)
		})
	}
}
