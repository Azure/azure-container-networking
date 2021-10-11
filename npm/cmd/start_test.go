package main

import (
	"testing"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	k8sversion "k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInitLogging(t *testing.T) {
	expectedLogPath := log.LogPath
	err := initLogging()
	require.NoError(t, err)
	require.Equal(t, expectedLogPath, log.GetLogDirectory())
}

func TestK8sServerVersion(t *testing.T) {
	// NPM has break behavior change from k8s version >= 1.11.
	// Thus, util.IsNewNwPolicyVerFlag flag is set based on running K8s version.
	tests := []struct {
		name             string
		info             *k8sversion.Info
		wantPanic        bool
		isNewNwPolicyVer bool
	}{
		{
			name: "Test higher version (>1.11)",
			info: &k8sversion.Info{
				Major:      "1.20",
				Minor:      "2",
				GitVersion: "v1.20.2",
			},
			wantPanic:        false,
			isNewNwPolicyVer: true,
		},
		{
			name: "Test equal version (1.11)",
			info: &k8sversion.Info{
				Major:      "1.11",
				Minor:      "0",
				GitVersion: "v1.11",
			},
			wantPanic:        false,
			isNewNwPolicyVer: true,
		},
		{
			name: "Test lower version (<1.11)",
			info: &k8sversion.Info{
				Major:      "1.10",
				Minor:      "1",
				GitVersion: "v1.10.1",
			},
			wantPanic:        false,
			isNewNwPolicyVer: false,
		},
		{
			name: "Test lower version (<1.11)",
			info: &k8sversion.Info{
				Major:      "0",
				Minor:      "0",
				GitVersion: "v0.0",
			},
			wantPanic:        false,
			isNewNwPolicyVer: false,
		},
		{
			name: "Test wrong minus version",
			info: &k8sversion.Info{
				Major:      "-1.11",
				Minor:      "0",
				GitVersion: "v-1.11",
			},
			wantPanic: true,
		},
		{
			name: "Test wrong alphabet version",
			info: &k8sversion.Info{
				Major:      "ab",
				Minor:      "cc",
				GitVersion: "vab.cc",
			},
			wantPanic: true,
		},
		{
			name: "Test wrong alphabet version",
			info: &k8sversion.Info{
				Major:      "1.1",
				Minor:      "cc",
				GitVersion: "v1.1.cc",
			},
			wantPanic: true,
		},
	}

	fc := fake.NewSimpleClientset()
	for _, tt := range tests {
		tt := tt
		fc.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = tt.info
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				require.Panics(t, func() {
					k8sServerVersion(fc)
				})
			} else {
				require.NotPanics(t, func() {
					got := k8sServerVersion(fc)
					require.Equal(t, got, tt.info)
					require.Equal(t, util.IsNewNwPolicyVerFlag, tt.isNewNwPolicyVer)
				})
			}
		})
	}
}
