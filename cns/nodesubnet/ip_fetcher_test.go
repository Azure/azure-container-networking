package nodesubnet_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/Azure/azure-container-networking/cns/nodesubnet"
	"github.com/Azure/azure-container-networking/nmagent"
)

// Mock client that simply consumes fetched IPs
type TestConsumer struct {
	consumeCount     int32
	secondaryIPCount int32
}

// FetchConsumeCount atomically fetches the consume count
func (c *TestConsumer) FetchConsumeCount() int32 {
	return c.consumeCount
}

// FetchSecondaryIPCount atomically fetches the last IP count
func (c *TestConsumer) FetchSecondaryIPCount() int32 {
	return c.consumeCount
}

// UpdateConsumeCount atomically updates the consume count
func (c *TestConsumer) updateCounts(ipCount int32) {
	c.consumeCount++
	c.secondaryIPCount = ipCount
}

// Mock IP update
func (c *TestConsumer) UpdateIPsForNodeSubnet(ips []netip.Addr) error {
	c.updateCounts(int32(len(ips)))
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
	fetcher := nodesubnet.NewIPFetcher(&TestClient{}, consumerPtr, 0, 0)
	err := fetcher.ProcessInterfaces(nmagent.Interfaces{})
	if err != nil {
		t.Error("Error processing empty interfaces")
	}

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
	fetcher := nodesubnet.NewIPFetcher(&TestClient{}, consumerPtr, 0, 0)
	err := fetcher.ProcessInterfaces(interfaces)
	if err != nil {
		t.Error("Error processing interfaces")
	}

	// 1 consume to be called
	if consumerPtr.FetchConsumeCount() != 1 {
		t.Error("Consume expected to be called, but not called")
	}

	// 1 consume to be called
	if consumerPtr.FetchSecondaryIPCount() != 1 {
		t.Error("Wrong number of secondary IPs ", consumerPtr.FetchSecondaryIPCount())
	}
}
