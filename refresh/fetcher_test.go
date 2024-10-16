package refresh_test

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"testing"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/refresh"
)

// Mock client that simply tracks if refresh has been called
type TestClient struct {
	refreshCount int
	responses    []nmagent.Interfaces
	mu           sync.Mutex
}

// FetchRefreshCount atomically fetches the refresh count
func (c *TestClient) FetchRefreshCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refreshCount
}

// UpdateRefreshCount atomically updates the refresh count
func (c *TestClient) UpdateRefreshCount() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshCount++
}

// Mock refresh
func (c *TestClient) GetInterfaceIPInfo(_ context.Context) (nmagent.Interfaces, error) {
	defer c.UpdateRefreshCount()

	if c.refreshCount >= len(c.responses) {
		return c.responses[len(c.responses)-1], nil
	}

	return c.responses[c.refreshCount], nil
}

var _ nodesubnet.InterfaceRetriever = &TestClient{}

// Mock client that simply consumes fetched IPs
type TestConsumer struct {
	consumeCount int
	mu           sync.Mutex
}

// FetchConsumeCount atomically fetches the consume count
func (c *TestConsumer) FetchConsumeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.consumeCount
}

// UpdateConsumeCount atomically updates the consume count
func (c *TestConsumer) UpdateConsumeCount() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consumeCount++
}

// Mock IP update
func (c *TestConsumer) ConsumeInterfaces(intfs nmagent.Interfaces) error {
	fmt.Printf("Consumed interfaces: %v\n", intfs)
	c.UpdateConsumeCount()
	return nil
}

func TestRefresh(t *testing.T) {
	clientPtr := &TestClient{
		refreshCount: 0,
		responses: []nmagent.Interfaces{
			{
				Entries: []nmagent.Interface{
					{
						MacAddress: nmagent.MACAddress{0x00, 0x0D, 0x3A, 0xF9, 0xDC, 0xA6},
						IsPrimary:  true,
						InterfaceSubnets: []nmagent.InterfaceSubnet{
							{
								Prefix: "10.240.0.0/16",
								IPAddress: []nmagent.NodeIP{
									{
										Address:   nmagent.IPAddress(netip.AddrFrom4([4]byte{10, 240, 0, 5})),
										IsPrimary: true,
									},
									{
										Address:   nmagent.IPAddress(netip.AddrFrom4([4]byte{10, 240, 0, 6})),
										IsPrimary: false,
									},
								},
							},
						},
					},
				},
			},
			{
				Entries: []nmagent.Interface{
					{
						MacAddress: nmagent.MACAddress{0x00, 0x0D, 0x3A, 0xF9, 0xDC, 0xA6},
						IsPrimary:  true,
						InterfaceSubnets: []nmagent.InterfaceSubnet{
							{
								Prefix: "10.240.0.0/16",
								IPAddress: []nmagent.NodeIP{
									{
										Address:   nmagent.IPAddress(netip.AddrFrom4([4]byte{10, 240, 0, 5})),
										IsPrimary: true,
									},
								},
							},
						},
					},
				},
			},
		},
		mu: sync.Mutex{},
	}

	consumerPtr := &TestConsumer{}
	fetcher := refresh.NewFetcher[nmagent.Interfaces](clientPtr.GetInterfaceIPInfo, 0, 0, consumerPtr.ConsumeInterfaces, logger.Log)
	ticker := refresh.NewMockTickProvider()
	fetcher.SetTicker(ticker)
	ctx, cancel := testContext(t)
	defer cancel()
	fetcher.Start(ctx)
	ticker.Tick() // Trigger a refresh
	ticker.Tick() // This tick will be read only after previous refresh is done
	ticker.Tick() // This call will block until the prevous tick is read

	// At least 2 refreshes - one initial and one after the first tick should be done
	if clientPtr.FetchRefreshCount() < 2 {
		t.Error("Not enough refreshes")
	}

	// Exactly 2 consumes - one initial and one after the first tick should be done (responses are different).
	// Then no more, since the response is unchanged
	if consumerPtr.FetchConsumeCount() != 2 {
		t.Error("Exactly two consumes expected (for two different responses)")
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
