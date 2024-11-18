package telemetryclient

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestClient(t *testing.T) {
	emptyClient := NewTelemetryClient(nil)

	// an empty client should not cause panics
	require.NotPanics(t, func() { emptyClient.SendEvent("no errors") })

	require.NotPanics(t, func() { emptyClient.DisconnectTelemetry() })

	require.NotPanics(t, func() { emptyClient.sendLog("no errors") })

	require.NotPanics(t, func() { emptyClient.sendTelemetry("no errors", "") })

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// should not panic if connecting telemetry fails or succeeds
	require.NotPanics(t, func() { emptyClient.ConnectTelemetry(logger) })

	// should set logger during connection
	require.Equal(t, logger, emptyClient.logger)
}
