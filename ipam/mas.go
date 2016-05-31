// Copyright Microsoft Corp.
// All rights reserved.

package ipam

import (
	"encoding/json"
	"net"
	"net/http"
	"time"
)

const (
	// Host URL to query.
	masQueryUrl = "http://169.254.169.254:6642/ListNetwork"

	// Minimum delay between consecutive polls.
	masDefaultMinPollPeriod = 30 * time.Second
)

// Microsoft Azure Stack IPAM configuration source.
type masSource struct {
	name          string
	sink          configSink
	lastRefresh   time.Time
	minPollPeriod time.Duration
}

// MAS host agent JSON object format.
type jsonObject struct {
	Isolation string
	IPs       []struct {
		IP              string
		IsolationId     string
		Mask            string
		DefaultGateways []string
		DnsServers      []string
	}
}

// Creates the MAS source.
func newMasSource(sink configSink) (*masSource, error) {
	return &masSource{
		name:          "MAS",
		sink:          sink,
		minPollPeriod: masDefaultMinPollPeriod,
	}, nil
}

// Starts the MAS source.
func (s *masSource) start() error {
	return nil
}

// Stops the MAS source.
func (s *masSource) stop() {
	return
}

// Refreshes configuration.
func (s *masSource) refresh() error {

	// Refresh only if enough time has passed since the last poll.
	if time.Since(s.lastRefresh) < s.minPollPeriod {
		return nil
	}
	s.lastRefresh = time.Now()

	// Configure the local default address space.
	local, err := newAddressSpace(localDefaultAddressSpaceId, localScope)
	if err != nil {
		return err
	}

	// Fetch configuration.
	resp, err := http.Get(masQueryUrl)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// Decode JSON object.
	var obj jsonObject
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&obj)
	if err != nil {
		return err
	}

	// Add the IP addresses to the local address space.
	for _, v := range obj.IPs {
		address := net.ParseIP(v.IP)
		subnet := net.IPNet{
			IP:   net.ParseIP(v.IP),
			Mask: net.IPMask(net.ParseIP(v.Mask)),
		}

		ap, err := local.newAddressPool("eth0", &subnet)
		if err != nil && err != errAddressExists {
			return err
		}

		_, err = ap.newAddressRecord(&address)
		if err != nil {
			return err
		}
	}

	// Set the local address space as active.
	s.sink.setAddressSpace(local)

	return nil
}
