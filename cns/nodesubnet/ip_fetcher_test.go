package nodesubnet_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/nmagent"
)

// Mock client that simply consumes fetched IPs
type TestConsumer struct {
	consumeCount     int
	secondaryIPCount int
}

// FetchConsumeCount atomically fetches the consume count
func (c *TestConsumer) FetchConsumeCount() int {
	return c.consumeCount
}

// FetchSecondaryIPCount atomically fetches the last IP count
func (c *TestConsumer) FetchSecondaryIPCount() int {
	return c.consumeCount
}

// UpdateConsumeCount atomically updates the consume count
func (c *TestConsumer) updateCounts(ipCount int) {
	c.consumeCount++
	c.secondaryIPCount = ipCount
}

// Mock IP update
func (c *TestConsumer) UpdateIPsForNodeSubnet(ips []netip.Addr) error {
	c.updateCounts(len(ips))
	return nil
}

var _ nodesubnet.IPConsumer = &TestConsumer{}

// Mock client that simply satisfies the interface
type TestClient struct{}

// Mock refresh
func (c *TestClient) GetInterfaceIPInfo(_ context.Context) (nmagent.Interfaces, error) {
	return nmagent.Interfaces{}, nil
}

func TestEmptyResponse(t *testing.T) {
	consumerPtr := &TestConsumer{}
	fetcher := nodesubnet.NewIPFetcher(&TestClient{}, consumerPtr, 0, 0, logger.Log)
	err := fetcher.ProcessInterfaces(nmagent.Interfaces{})
	checkErr(t, err, true)

	// No consumes, since the responses are empty
	if consumerPtr.FetchConsumeCount() > 0 {
		t.Error("Consume called unexpectedly, shouldn't be called since responses are empty")
	}
}

func TestFlatten(t *testing.T) {
	interfaces := nmagent.Interfaces{
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
	}
	consumerPtr := &TestConsumer{}
	fetcher := nodesubnet.NewIPFetcher(&TestClient{}, consumerPtr, 0, 0, logger.Log)
	err := fetcher.ProcessInterfaces(interfaces)
	checkErr(t, err, false)

	// 1 consume to be called
	if consumerPtr.FetchConsumeCount() != 1 {
		t.Error("Consume expected to be called, but not called")
	}

	// 1 consume to be called
	if consumerPtr.FetchSecondaryIPCount() != 1 {
		t.Error("Wrong number of secondary IPs ", consumerPtr.FetchSecondaryIPCount())
	}
}

// checkErr is an assertion of the presence or absence of an error
func checkErr(t *testing.T, err error, shouldErr bool) {
	t.Helper()
	if err != nil && !shouldErr {
		t.Fatal("unexpected error: err:", err)
	}

	if err == nil && shouldErr {
		t.Fatal("expected error but received none")
	}
}

func init() {
	logger.InitLogger("testlogs", 0, 0, "./")
}
