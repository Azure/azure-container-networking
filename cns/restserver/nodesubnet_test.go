package restserver_test

import (
	"context"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/store"
)

// getMockStore creates a mock KeyValueStore with some endpoint state
func getMockStore() store.KeyValueStore {
	mockStore := store.NewMockStore("")
	endpointState := map[string]*restserver.EndpointInfo{
		"12e65d89e58cb23c784e97840cf76866bfc9902089bdc8e87e9f64032e312b0b": {
			PodName:      "coredns-54b69f46b8-ldmwr",
			PodNamespace: "kube-system",
			IfnameToIPMap: map[string]*restserver.IPInfo{
				"eth0": {
					IPv4: []net.IPNet{
						{
							IP:   net.IPv4(10, 0, 0, 52),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
			},
		},
		"1fc5176913a3a1a7facfb823dde3b4ded404041134fef4f4a0c8bba140fc0413": {
			PodName:      "load-test-7f7d49687d-wxc9p",
			PodNamespace: "load-test",
			IfnameToIPMap: map[string]*restserver.IPInfo{
				"eth0": {
					IPv4: []net.IPNet{
						{
							IP:   net.IPv4(10, 0, 0, 63),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
			},
		},
	}

	err := mockStore.Write(restserver.EndpointStoreKey, endpointState)
	if err != nil {
		return nil
	}
	return mockStore
}

// Mock implementation of CNIConflistGenerator
type MockCNIConflistGenerator struct {
	GenerateCalled chan struct{}
}

func (m *MockCNIConflistGenerator) Generate() error {
	close(m.GenerateCalled)
	return nil
}

func (m *MockCNIConflistGenerator) Close() error {
	// Implement the Close method logic here if needed
	return nil
}

// TestNodeSubnet tests initialization of NodeSubnet with endpoint info, and verfies that
// the conflist is generated after fetching secondary IPs
func TestNodeSubnet(t *testing.T) {
	podInfoByIPProvider, err := cnireconciler.NewCNSPodInfoProvider(getMockStore())
	if err != nil {
		t.Fatalf("NewCNSPodInfoProvider returned an error: %v", err)
	}

	// create a real HTTPRestService object
	mockCNIConflistGenerator := &MockCNIConflistGenerator{
		GenerateCalled: make(chan struct{}),
	}
	service := restserver.GetRestServiceObjectForNodeSubnetTest(t, mockCNIConflistGenerator)
	ctx, cancel := testContext(t)
	defer cancel()

	err = service.InitializeNodeSubnet(ctx, podInfoByIPProvider)
	if err != nil {
		t.Fatalf("InitializeNodeSubnet returned an error: %v", err)
	}

	expectedIPs := map[string]types.IPState{
		"10.0.0.52": types.Assigned,
		"10.0.0.63": types.Assigned,
	}

	checkIPassignment(t, service, expectedIPs)

	service.StartNodeSubnet(ctx)

	if service.GetNodesubnetIPFetcher() == nil {
		t.Fatal("NodeSubnetIPFetcher is not initialized")
	}

	select {
	case <-ctx.Done():
		t.Errorf("test context done - %s", ctx.Err())
		return
	case <-mockCNIConflistGenerator.GenerateCalled:
		break
	}

	expectedIPs["10.0.0.45"] = types.Available
	checkIPassignment(t, service, expectedIPs)
}

// checkIPassignment checks whether the IP assignment state in the HTTPRestService object matches expectation
func checkIPassignment(t *testing.T, service *restserver.HTTPRestService, expectedIPs map[string]types.IPState) {
	if len(service.PodIPConfigState) != len(expectedIPs) {
		t.Fatalf("expected 2 entries in PodIPConfigState, got %d", len(service.PodIPConfigState))
	}

	for ip := range service.GetPodIPConfigState() {
		config := service.GetPodIPConfigState()[ip]
		if assignmentState, exists := expectedIPs[ip]; !exists {
			t.Fatalf("unexpected IP %s in PodIPConfigState", ip)
		} else if config.GetState() != assignmentState {
			t.Fatalf("expected state 'Assigned' for IP %s, got %s", ip, config.GetState())
		}
	}
}

// testContext creates a context from the provided testing.T that will be
// canceled if the test suite is terminated.
func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	if deadline, ok := t.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}
	return context.WithCancel(context.Background())
}

func init() {
	logger.InitLogger("testlogs", 0, 0, "./")
}
