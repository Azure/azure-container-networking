package main

import (
	"testing"

	"github.com/Azure/azure-container-networking/log"
	"github.com/stretchr/testify/require"
)

func TestInitLogging(t *testing.T) {
	expectedLogPath := log.LogPath
	err := initLogging()
	require.NoError(t, err)
	require.Equal(t, expectedLogPath, log.GetLogDirectory())
}
