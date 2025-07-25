package telemetry

import (
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const telemetryConfig = "azure-vnet-telemetry.config"

func createTBServer(t *testing.T) (*TelemetryBuffer, func()) {
	tbServer := NewTelemetryBuffer(nil)
	err := tbServer.StartServer()
	require.NoError(t, err)

	return tbServer, func() {
		tbServer.Close()
		err := tbServer.Cleanup(FdName)
		require.Error(t, err)
	}
}

func TestStartServer(t *testing.T) {
	_, closeTBServer := createTBServer(t)
	defer closeTBServer()

	secondTBServer := NewTelemetryBuffer(nil)
	err := secondTBServer.StartServer()
	require.Error(t, err)
}

func TestConnect(t *testing.T) {
	_, closeTBServer := createTBServer(t)
	defer closeTBServer()

	logger := log.TelemetryLogger.With(zap.String("component", "cni-telemetry"))
	tbClient := NewTelemetryBuffer(logger)
	err := tbClient.Connect()
	require.NoError(t, err)

	tbClient.Close()
}

func TestServerConnClose(t *testing.T) {
	tbServer, closeTBServer := createTBServer(t)
	defer closeTBServer()

	tbClient := NewTelemetryBuffer(nil)
	err := tbClient.Connect()
	require.NoError(t, err)
	defer tbClient.Close()

	tbServer.Close()

	b := []byte("testdata")
	_, err = tbClient.Write(b)
	require.Error(t, err)
}

func TestClientConnClose(t *testing.T) {
	_, closeTBServer := createTBServer(t)
	defer closeTBServer()

	tbClient := NewTelemetryBuffer(nil)
	err := tbClient.Connect()
	require.NoError(t, err)
	tbClient.Close()
}

func TestCloseOnWriteError(t *testing.T) {
	tbServer, closeTBServer := createTBServer(t)
	defer closeTBServer()

	tbClient := NewTelemetryBuffer(nil)
	err := tbClient.Connect()
	require.NoError(t, err)
	defer tbClient.Close()

	data := []byte("{\"good\":1}")
	_, err = tbClient.Write(data)
	require.NoError(t, err)
	// need to wait for connection to populate in server
	time.Sleep(1 * time.Second)
	tbServer.mutex.Lock()
	conns := tbServer.connections
	tbServer.mutex.Unlock()
	require.Len(t, conns, 1)

	// the connection should be automatically closed on failure
	badData := []byte("} malformed json }}}")
	_, err = tbClient.Write(badData)
	require.NoError(t, err)
	time.Sleep(1 * time.Second)
	tbServer.mutex.Lock()
	conns = tbServer.connections
	tbServer.mutex.Unlock()
	require.Empty(t, conns)
}

func TestWrite(t *testing.T) {
	_, closeTBServer := createTBServer(t)
	defer closeTBServer()

	tbClient := NewTelemetryBuffer(nil)
	err := tbClient.Connect()
	require.NoError(t, err)
	defer tbClient.Close()

	tests := []struct {
		name    string
		data    []byte
		want    int
		wantErr bool
	}{
		{
			name:    "write",
			data:    []byte("{\"testdata\":1}"),
			want:    len("{\"testdata\":1}") + 1, // +1 due to Delimiter('\n)
			wantErr: false,
		},
		{
			name:    "write zero data",
			data:    []byte(""),
			want:    1, // +1 due to Delimiter('\n)
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := tbClient.Write(tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got, "Expected:%d but got:%d", tt.want, got)
		})
	}
}

func TestReadConfigFile(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		want     TelemetryConfig
		wantErr  bool
	}{
		{
			name:     "read existing file",
			fileName: telemetryConfig,
			want: TelemetryConfig{
				ReportToHostIntervalInSeconds: time.Duration(30),
				RefreshTimeoutInSecs:          15,
				BatchIntervalInSecs:           15,
				BatchSizeInBytes:              16384,
			},
			wantErr: false,
		},
		{
			name:     "read non-existing file",
			fileName: "non-existing-file",
			want:     TelemetryConfig{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadConfigFile(tt.fileName)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestStartTelemetryService(t *testing.T) {
	tb := NewTelemetryBuffer(nil)
	err := tb.StartTelemetryService("", nil)
	require.Error(t, err)
}

// TestExtraneousClose checks that closing potentially multiple times after a failed connect won't panic
func TestExtraneousClose(_ *testing.T) {
	tb := NewTelemetryBuffer(nil)

	tb.Close()
	tb.Close()

	tb.ConnectToTelemetry()

	tb.Close()
	tb.Close()

	tb = NewTelemetryBuffer(nil)
	tb.ConnectToTelemetryService(telemetryNumberRetries, telemetryWaitTimeInMilliseconds)

	tb.Close()
	tb.Close()
}
