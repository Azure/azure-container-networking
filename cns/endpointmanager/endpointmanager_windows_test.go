//go:build windows

package endpointmanager

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIfName        = "eth0"
	testEndpointID    = "ep-1"
	testHNSNetworkID  = "net-1"
	testContainerID   = "c1"
	testLookupHNSID   = "ep-from-ip"
	testExplicitHNSID = "ep-123"
)

var (
	errGetEndpointFailed = errors.New("endpointmanager test: get endpoint failed")
	errLookupFailed      = errors.New("endpointmanager test: lookup failed")
	errDeleteFailed      = errors.New("endpointmanager test: delete failed")
	errCNSReleaseFailed  = errors.New("endpointmanager test: cns release failed")
)

func TestMain(m *testing.M) {
	logger.InitLogger("endpointmanager-test", 0, 0, "./") //nolint:staticcheck // tests still exercise code using the deprecated global logger
	os.Exit(m.Run())
}

type fakeReleaseIPsClient struct {
	getResp    *restserver.GetEndpointResponse
	getErr     error
	releaseErr error

	getCalls        int
	getContainerIDs []string
	releaseCalls    int
	releaseReq      cns.IPConfigsRequest
}

func (f *fakeReleaseIPsClient) ReleaseIPs(_ context.Context, req cns.IPConfigsRequest) error {
	f.releaseCalls++
	f.releaseReq = req
	return f.releaseErr
}

func (f *fakeReleaseIPsClient) GetEndpoint(_ context.Context, containerID string) (*restserver.GetEndpointResponse, error) {
	f.getCalls++
	f.getContainerIDs = append(f.getContainerIDs, containerID)
	return f.getResp, f.getErr
}

type fakeHNS struct {
	// return values
	lookupID     string
	lookupErr    error
	deleteEPErr  error
	deleteNetErr error

	// recorded calls
	lookupCalls  int
	lookupV4     [][]net.IPNet
	lookupV6     [][]net.IPNet
	deleteEPIDs  []string
	deleteNetIDs []string
}

func (f *fakeHNS) GetHNSEndpointbyIP(ipv4, ipv6 []net.IPNet) (string, error) {
	f.lookupCalls++
	f.lookupV4 = append(f.lookupV4, ipv4)
	f.lookupV6 = append(f.lookupV6, ipv6)
	return f.lookupID, f.lookupErr
}

func (f *fakeHNS) DeleteHNSEndpointbyID(hnsEndpointID string) error {
	f.deleteEPIDs = append(f.deleteEPIDs, hnsEndpointID)
	return f.deleteEPErr
}

func (f *fakeHNS) DeleteNetworkByIDHnsV2(networkID string) error {
	f.deleteNetIDs = append(f.deleteNetIDs, networkID)
	return f.deleteNetErr
}

// swapHNS replaces the package-level hns var with fake for the duration of the test.
func swapHNS(t *testing.T, fake hnsEndpointClient) {
	t.Helper()
	prev := hns
	hns = fake
	t.Cleanup(func() { hns = prev })
}

func endpointResp(entries map[string]*restserver.IPInfo) *restserver.GetEndpointResponse {
	return &restserver.GetEndpointResponse{
		EndpointInfo: restserver.EndpointInfo{IfnameToIPMap: entries},
	}
}

func TestReleaseIPs_GetEndpointError_StillReleasesIPs(t *testing.T) {
	fake := &fakeHNS{}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{getErr: errGetEndpointFailed}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{InfraContainerID: testContainerID})

	require.NoError(t, err)
	assert.Equal(t, 1, cli.getCalls)
	assert.Equal(t, []string{testContainerID}, cli.getContainerIDs)
	assert.Equal(t, 1, cli.releaseCalls)
	assert.Equal(t, 0, fake.lookupCalls)
	assert.Empty(t, fake.deleteEPIDs)
	assert.Empty(t, fake.deleteNetIDs)
}

func TestReleaseIPs_HnsEndpointIDPresent_DeletesByID(t *testing.T) {
	fake := &fakeHNS{}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {HnsEndpointID: testExplicitHNSID},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{InfraContainerID: testContainerID})

	require.NoError(t, err)
	assert.Equal(t, 0, fake.lookupCalls, "GetHNSEndpointbyIP must not be called when HnsEndpointID is set")
	assert.Equal(t, []string{testExplicitHNSID}, fake.deleteEPIDs)
	assert.Empty(t, fake.deleteNetIDs)
	assert.Equal(t, 1, cli.releaseCalls)
}

func TestReleaseIPs_HnsEndpointIDEmpty_LooksUpByIP(t *testing.T) {
	fake := &fakeHNS{lookupID: testLookupHNSID}
	swapHNS(t, fake)
	v4 := []net.IPNet{{IP: net.IPv4(10, 0, 0, 1), Mask: net.CIDRMask(24, 32)}}
	v6 := []net.IPNet{{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)}}
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {IPv4: v4, IPv6: v6},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{InfraContainerID: testContainerID})

	require.NoError(t, err)
	require.Equal(t, 1, fake.lookupCalls)
	assert.Equal(t, v4, fake.lookupV4[0])
	assert.Equal(t, v6, fake.lookupV6[0])
	assert.Equal(t, []string{testLookupHNSID}, fake.deleteEPIDs)
	assert.Equal(t, 1, cli.releaseCalls)
}

func TestReleaseIPs_LookupByIPError_StillReleasesIPs(t *testing.T) {
	fake := &fakeHNS{lookupErr: errLookupFailed}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {IPv4: []net.IPNet{{IP: net.IPv4(10, 0, 0, 1), Mask: net.CIDRMask(24, 32)}}},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{})

	require.NoError(t, err)
	assert.Empty(t, fake.deleteEPIDs, "delete must not run when lookup failed")
	assert.Equal(t, 1, cli.releaseCalls)
}

func TestReleaseIPs_DeleteEndpointError_StillReleasesIPs(t *testing.T) {
	fake := &fakeHNS{deleteEPErr: errDeleteFailed}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {HnsEndpointID: testEndpointID},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{})

	require.NoError(t, err)
	assert.Equal(t, []string{testEndpointID}, fake.deleteEPIDs)
	assert.Empty(t, fake.deleteNetIDs, "network delete must not run after endpoint delete failed")
	assert.Equal(t, 1, cli.releaseCalls)
}

func TestReleaseIPs_DelegatedVMNIC_DeletesNetwork(t *testing.T) {
	fake := &fakeHNS{}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {HnsEndpointID: testEndpointID, HnsNetworkID: testHNSNetworkID, NICType: cns.DelegatedVMNIC},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{})

	require.NoError(t, err)
	assert.Equal(t, []string{testEndpointID}, fake.deleteEPIDs)
	assert.Equal(t, []string{testHNSNetworkID}, fake.deleteNetIDs)
}

func TestReleaseIPs_NonDelegated_SkipsNetworkDelete(t *testing.T) {
	fake := &fakeHNS{}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {HnsEndpointID: testEndpointID, HnsNetworkID: testHNSNetworkID, NICType: cns.InfraNIC},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{})

	require.NoError(t, err)
	assert.Equal(t, []string{testEndpointID}, fake.deleteEPIDs)
	assert.Empty(t, fake.deleteNetIDs, "non-delegated NICs must not trigger network deletion")
}

func TestReleaseIPs_DelegatedVMNIC_EmptyNetworkID_SkipsNetworkDelete(t *testing.T) {
	fake := &fakeHNS{}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {HnsEndpointID: testEndpointID, NICType: cns.DelegatedVMNIC},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{})

	require.NoError(t, err)
	assert.Empty(t, fake.deleteNetIDs)
}

func TestReleaseIPs_MultipleInterfaces_AllProcessed(t *testing.T) {
	fake := &fakeHNS{}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp: endpointResp(map[string]*restserver.IPInfo{
			testIfName: {HnsEndpointID: "ep-a"},
			"eth1":     {HnsEndpointID: "ep-b", HnsNetworkID: "net-b", NICType: cns.DelegatedVMNIC},
		}),
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ep-a", "ep-b"}, fake.deleteEPIDs)
	assert.Equal(t, []string{"net-b"}, fake.deleteNetIDs)
}

func TestReleaseIPs_ReleaseError_Wrapped(t *testing.T) {
	fake := &fakeHNS{}
	swapHNS(t, fake)
	cli := &fakeReleaseIPsClient{
		getResp:    endpointResp(map[string]*restserver.IPInfo{}),
		releaseErr: errCNSReleaseFailed,
	}
	em := WithPlatformReleaseIPsManager(cli)

	err := em.ReleaseIPs(context.Background(), cns.IPConfigsRequest{InfraContainerID: testContainerID})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to release IP from CNS")
	require.ErrorIs(t, err, errCNSReleaseFailed)
	assert.Equal(t, 1, cli.releaseCalls)
}
