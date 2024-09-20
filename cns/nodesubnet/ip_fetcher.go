package nodesubnet

import (
	"context"
	"log"
	"net/netip"
	"time"

	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/pkg/errors"
)

var ErrRefreshSkipped = errors.New("refresh skipped due to throttling")

// InterfaceRetriever is an interface is implemented by the NMAgent Client, and also a mock client for testing.
type InterfaceRetriever interface {
	GetInterfaceIPInfo(ctx context.Context) (nmagent.Interfaces, error)
}

type IPFetcher struct {
	// Node subnet state
	secondaryIPQueryInterval   time.Duration // Minimum time between secondary IP fetches
	secondaryIPLastRefreshTime time.Time     // Time of last secondary IP fetch

	ipFectcherClient InterfaceRetriever
}

func NewIPFetcher(nmaClient InterfaceRetriever, queryInterval time.Duration) *IPFetcher {
	return &IPFetcher{
		ipFectcherClient:         nmaClient,
		secondaryIPQueryInterval: queryInterval,
	}
}

func (c *IPFetcher) RefreshSecondaryIPsIfNeeded(ctx context.Context) (ips []netip.Addr, err error) {
	// If secondaryIPQueryInterval has elapsed since the last fetch, fetch secondary IPs
	if time.Since(c.secondaryIPLastRefreshTime) < c.secondaryIPQueryInterval {
		return nil, ErrRefreshSkipped
	}

	c.secondaryIPLastRefreshTime = time.Now()
	response, err := c.ipFectcherClient.GetInterfaceIPInfo(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting interface IPs")
	}

	res := flattenIPListFromResponse(&response)
	return res, nil
}

// Get the list of secondary IPs from fetched Interfaces
func flattenIPListFromResponse(resp *nmagent.Interfaces) (res []netip.Addr) {
	// For each interface...
	for _, intf := range resp.Entries {
		if !intf.IsPrimary {
			continue
		}

		// For each subnet on the interface...
		for _, s := range intf.InterfaceSubnets {
			addressCount := 0
			// For each address in the subnet...
			for _, a := range s.IPAddress {
				// Primary addresses are reserved for the host.
				if a.IsPrimary {
					continue
				}

				res = append(res, netip.Addr(a.Address))
				addressCount++
			}
			log.Printf("Got %d addresses from subnet %s", addressCount, s.Prefix)
		}
	}

	return res
}
