package metric

import (
	"crypto/md5" //nolint:gosec // used for checksum
	"encoding/json"
	"testing"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCNSConfigSnapshotEvent(t *testing.T) {
	logger.InitLogger("testlogs", 0, 0, "./")

	config, err := configuration.ReadConfig("../configuration/testdata/good.json")
	require.NoError(t, err)

	event, err := createCNSConfigSnapshotEvent(config)
	require.NoError(t, err)

	bb, err := json.Marshal(config) //nolint:musttag // no tag needed for config
	require.NoError(t, err)

	cs := md5.Sum(bb) //nolint:gosec // used for checksum
	csStr := string(cs[:])

	expected := aitelemetry.Event{
		EventName:  logger.ConfigSnapshotMetricsStr,
		ResourceID: csStr,
		Properties: map[string]string{
			logger.CNSConfigPropertyStr:            string(bb),
			logger.CNSConfigMD5CheckSumPropertyStr: csStr,
		},
	}

	assert.Equal(t, expected, event)
}
