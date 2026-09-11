//go:build !windows
// +build !windows

package main

import (
	"testing"

	acn "github.com/Azure/azure-container-networking/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleServiceActionOnNonWindows(t *testing.T) {
	t.Run("install should fail", func(t *testing.T) {
		exit, err := handleServiceAction(acn.OptServiceInstall)
		require.ErrorIs(t, err, errServiceInstallationUnsupported)
		assert.False(t, exit)
	})

	t.Run("uninstall should fail", func(t *testing.T) {
		exit, err := handleServiceAction(acn.OptServiceUninstall)
		require.ErrorIs(t, err, errServiceUninstallationUnsupported)
		assert.False(t, exit)
	})

	t.Run("run should fail", func(t *testing.T) {
		exit, err := handleServiceAction(acn.OptServiceRun)
		require.ErrorIs(t, err, errServiceRunUnsupported)
		assert.False(t, exit)
	})

	t.Run("no action should continue startup", func(t *testing.T) {
		exit, err := handleServiceAction("")
		require.NoError(t, err)
		assert.False(t, exit)
	})
}
