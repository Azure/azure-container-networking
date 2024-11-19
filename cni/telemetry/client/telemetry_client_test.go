package telemetryclient

import (
	"errors"
	"regexp"
	"testing"

	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var errMockTelemetryClient = errors.New("mock telemetry client error")

func TestClient(t *testing.T) {
	allowedErrorMsg := regexp.MustCompile(`^\[\d+\] mock telemetry client error`)
	allowedEventMsg := regexp.MustCompile(`^\[\d+\] telemetry event`)

	emptyClient := NewTelemetryClient()

	// an empty client should not cause panics
	require.NotPanics(t, func() { emptyClient.SendEvent("no errors") })

	require.NotPanics(t, func() { emptyClient.SendError(errMockTelemetryClient) })

	require.NotPanics(t, func() { emptyClient.DisconnectTelemetry() })

	require.NotPanics(t, func() { emptyClient.sendLog("no errors") })

	require.NotPanics(t, func() { emptyClient.sendEvent("no errors") })

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	// should not panic if connecting telemetry fails or succeeds
	require.NotPanics(t, func() { emptyClient.ConnectTelemetry(logger) })

	// should set logger during connection
	require.Equal(t, logger, emptyClient.logger)

	// for testing, we create a new telemetry buffer and assign it
	emptyClient.tb = &telemetry.TelemetryBuffer{}

	// test sending error
	require.NotPanics(t, func() { emptyClient.SendError(errMockTelemetryClient) })
	require.Regexp(t, allowedErrorMsg, emptyClient.Settings().EventMessage)

	// test sending event, error is empty
	require.NotPanics(t, func() { emptyClient.SendEvent("telemetry event") })
	require.Regexp(t, allowedEventMsg, emptyClient.Settings().EventMessage)
	require.Equal(t, "", emptyClient.Settings().ErrorMessage)

	// test sending aimetrics doesn't panic...
	require.NotPanics(t, func() { emptyClient.SendMetric(&telemetry.AIMetric{}) })
	// ...and doesn't affect the cni report
	require.Regexp(t, allowedEventMsg, emptyClient.Settings().EventMessage)
	require.Equal(t, "", emptyClient.Settings().ErrorMessage)
}
