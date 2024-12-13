package metric

import (
	"encoding/json"
	"testing"

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

	assert.Equal(t, logger.ConfigSnapshotMetricsStr, event.EventName)
	assert.NotEmpty(t, event.ResourceID)
	assert.Contains(t, event.Properties[logger.CNSConfigPropertyStr], "\"TLSPort\":\"10091\"")

	eventConfig := &configuration.CNSConfig{}
	err = json.Unmarshal([]byte(event.Properties[logger.CNSConfigPropertyStr]), eventConfig) //nolint:musttag // no tag needed for config
	require.NoError(t, err)
	assert.EqualValues(t, config, eventConfig)
}
