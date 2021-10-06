package cni

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/store"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Run tests.
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestPluginSafeToRemoveLock(t *testing.T) {
	tests := []struct {
		name        string
		plugin      Plugin
		processName string
		wantIsSafe  bool
		wantErr     bool
	}{
		{
			name: "Safe to remove lock-true. Process name does not match",
			plugin: Plugin{
				Plugin: &common.Plugin{
					Name:    "cni",
					Version: "0.3.0",
					Store:   store.NewMockStore("testfiles/processfound.lock"),
				},
				version: "0.3.0",
			},
			processName: "azure-vnet",
			wantIsSafe:  true,
			wantErr:     false,
		},
		{
			name: "Safe to remove lock-true. Process not running",
			plugin: Plugin{
				Plugin: &common.Plugin{
					Name:    "cni",
					Version: "0.3.0",
					Store:   store.NewMockStore("testfiles/processnotfound.lock"),
				},
				version: "0.3.0",
			},
			processName: "azure-vnet",
			wantIsSafe:  true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			isSafe, err := tt.plugin.IsSafeToRemoveLock(tt.processName)
			if tt.wantErr {
				require.Error(t, err)
				require.Equal(t, tt.wantIsSafe, isSafe)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantIsSafe, isSafe)
			}
		})
	}
}
