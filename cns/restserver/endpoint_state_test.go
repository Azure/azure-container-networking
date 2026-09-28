package restserver

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEndpointMACAddress = "00:11:22:33:44:55"

func TestCloneEndpointState(t *testing.T) {
	t.Parallel()
	state := map[string]*EndpointInfo{
		"container": {
			PodName:      "pod",
			PodNamespace: "namespace",
			IfnameToIPMap: map[string]*IPInfo{
				InfraInterfaceName: {
					IPv4:               []net.IPNet{{IP: net.IP{10, 0, 0, 1}, Mask: net.CIDRMask(24, 32)}},
					IPv6:               []net.IPNet{{IP: net.IP{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, Mask: net.CIDRMask(64, 128)}},
					HnsEndpointID:      "endpoint",
					HnsNetworkID:       "network",
					HostVethName:       "veth",
					MacAddress:         testEndpointMACAddress,
					NetworkContainerID: "nc",
					NICType:            cns.InfraNIC,
				},
				"nil": nil,
			},
		},
		"nil": nil,
	}
	cloned := cloneEndpointState(state)
	require.Equal(t, state, cloned)

	cloned["container"].PodName = "changed"
	cloned["container"].IfnameToIPMap[InfraInterfaceName].IPv4[0].IP[0] = 11
	cloned["container"].IfnameToIPMap[InfraInterfaceName].IPv4[0].Mask[0] = 0
	cloned["container"].IfnameToIPMap[InfraInterfaceName].IPv6[0].IP[0] = 0
	cloned["container"].IfnameToIPMap[InfraInterfaceName].IPv6[0].Mask[0] = 0
	delete(cloned["container"].IfnameToIPMap, "nil")
	delete(cloned, "nil")

	assert.Equal(t, "pod", state["container"].PodName)
	assert.Equal(t, byte(10), state["container"].IfnameToIPMap[InfraInterfaceName].IPv4[0].IP[0])
	assert.Equal(t, byte(255), state["container"].IfnameToIPMap[InfraInterfaceName].IPv4[0].Mask[0])
	assert.Equal(t, byte(0x20), state["container"].IfnameToIPMap[InfraInterfaceName].IPv6[0].IP[0])
	assert.Equal(t, byte(255), state["container"].IfnameToIPMap[InfraInterfaceName].IPv6[0].Mask[0])
	assert.Contains(t, state["container"].IfnameToIPMap, "nil")
	assert.Contains(t, state, "nil")
}

func BenchmarkCloneEndpointState(b *testing.B) {
	for _, endpoints := range []int{10, 150, 250} {
		b.Run(fmt.Sprintf("endpoints=%d", endpoints), func(b *testing.B) {
			state := make(map[string]*EndpointInfo, endpoints)
			for i := range endpoints {
				state[strconv.Itoa(i)] = &EndpointInfo{
					PodName:      "pod",
					PodNamespace: "namespace",
					IfnameToIPMap: map[string]*IPInfo{
						InfraInterfaceName: {
							IPv4: []net.IPNet{{IP: net.IP{10, 0, 0, 1}, Mask: net.CIDRMask(24, 32)}},
							IPv6: []net.IPNet{{IP: net.IP(netip.MustParseAddr("2001:db8::1").AsSlice()), Mask: net.CIDRMask(64, 128)}},
						},
					},
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				cloneEndpointState(state)
			}
		})
	}
}
