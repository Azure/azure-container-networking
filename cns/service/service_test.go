//go:build !windows
// +build !windows

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestServiceFunctionsOnNonWindows tests that service functions return appropriate errors on non-Windows platforms
func TestServiceFunctionsOnNonWindows(t *testing.T) {
	t.Run("installService should fail on non-Windows", func(t *testing.T) {
		err := installService()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supported on Windows")
	})

	t.Run("uninstallService should fail on non-Windows", func(t *testing.T) {
		err := uninstallService()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supported on Windows")
	})

	t.Run("runAsService should fail on non-Windows", func(t *testing.T) {
		err := runAsService()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supported on Windows")
	})

	t.Run("isWindowsService should return false on non-Windows", func(t *testing.T) {
		isService, err := isWindowsService()
		assert.NoError(t, err)
		assert.False(t, isService)
	})
}
