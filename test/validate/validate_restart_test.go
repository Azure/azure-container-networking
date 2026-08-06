package validate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldRestartKubeProxy(t *testing.T) {
	require.True(t, shouldRestartKubeProxy(false))
	require.False(t, shouldRestartKubeProxy(true))
}

func TestCanSkipEmptyRestartState(t *testing.T) {
	require.True(t, canSkipEmptyRestartState("linux", true))
	require.False(t, canSkipEmptyRestartState("windows", true))
	require.False(t, canSkipEmptyRestartState("windows", false))
}
