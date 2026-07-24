// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnifiedPodInfoByIPProvider(t *testing.T) {
	service := newAdapterTestService()
	provider := service.UnifiedPodInfoByIPProvider()
	_, err := provider.PodInfoByIP()
	require.Error(t, err)

	service.EndpointState = map[string]*EndpointInfo{
		"container-a": {
			PodName:      "pod-a",
			PodNamespace: "namespace-a",
			IfnameToIPMap: map[string]*IPInfo{
				"eth0": {
					IPv4:    []net.IPNet{{IP: net.IPv4(10, 0, 0, 4), Mask: net.CIDRMask(24, 32)}},
					NICType: cns.InfraNIC,
				},
			},
		},
	}
	service.setUnifiedStateAdapter(&durableStateAdapter{})
	pods, err := provider.PodInfoByIP()
	require.NoError(t, err)
	require.Contains(t, pods, "10.0.0.4")
	assert.Equal(t, cns.NewPodInfo("container-a", "container-a", "pod-a", "namespace-a"), pods["10.0.0.4"])
}
