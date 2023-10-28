// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/common"
	"go.uber.org/zap"
)

var logger = log.CNILogger.With(zap.String("component", "ipam"))

const (
	// Host URL to query.
	azureQueryUrl = "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
	// Minimum time interval between consecutive queries.
	azureQueryInterval = 10 * time.Second
	// http connection timeout
	httpConnectionTimeout = 10
	// http response header timeout
	responseHeaderTimeout = 10
)

// Microsoft Azure IPAM configuration source.
type azureSource struct {
	name          string
	sink          addressConfigSink
	queryUrl      string
	queryInterval time.Duration
	lastRefresh   time.Time
}

// Creates the Azure source.
func newAzureSource(options map[string]interface{}) (*azureSource, error) {
	queryUrl, _ := options[common.OptIpamQueryUrl].(string)
	if queryUrl == "" {
		queryUrl = azureQueryUrl
	}

	i, _ := options[common.OptIpamQueryInterval].(int)
	queryInterval := time.Duration(i) * time.Second
	if queryInterval == 0 {
		queryInterval = azureQueryInterval
	}

	return &azureSource{
		name:          "Azure",
		queryUrl:      queryUrl,
		queryInterval: queryInterval,
	}, nil
}

// Starts the Azure source.
func (s *azureSource) start(sink addressConfigSink) error {
	s.sink = sink
	return nil
}

// Stops the Azure source.
func (s *azureSource) stop() {
	s.sink = nil
}

// Refreshes configuration.
func (s *azureSource) refresh() error {
	// Refresh only if enough time has passed since the last query.
	if time.Since(s.lastRefresh) < s.queryInterval {
		return nil
	}
	s.lastRefresh = time.Now()

	// Query the list of local interfaces.
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	// Configure the local default address space.
	local, err := s.sink.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
	if err != nil {
		return err
	}

	httpClient := common.InitHttpClient(httpConnectionTimeout, responseHeaderTimeout)
	if httpClient == nil {
		logger.Error("Failed intializing http client")
		return fmt.Errorf("Error intializing http client")
	}

	logger.Info("Wireserver call to retrieve IP List", zap.String("queryUrl", s.queryUrl))
	// Fetch configuration.
	resp, err := httpClient.Get(s.queryUrl)
	if err != nil {
		logger.Error("wireserver call failed", zap.Error(err))
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("http return error code for wireserver call", zap.Any("response", resp))
		return fmt.Errorf("wireserver http error %+v", resp)
	}

	// Decode XML document.
	var doc common.XmlDocument
	decoder := xml.NewDecoder(resp.Body)
	err = decoder.Decode(&doc)
	if err != nil {
		return err
	}

	// For each interface...
	for _, i := range doc.Interface {
		ifName := ""
		priority := 0
		i.MacAddress = strings.ToLower(i.MacAddress)

		// Find the interface with the matching MacAddress.
		for _, iface := range interfaces {
			macAddr := strings.Replace(iface.HardwareAddr.String(), ":", "", -1)
			macAddr = strings.ToLower(macAddr)
			if macAddr == i.MacAddress || i.MacAddress == "*" {
				ifName = iface.Name

				// Prioritize secondary interfaces.
				if !i.IsPrimary {
					priority = 1
				}
				break
			}
		}

		// Skip if interface is not found.
		if ifName == "" {
			logger.Info("Failed to find interface with", zap.String("MAC Address", i.MacAddress))
			continue
		}

		// For each subnet on the interface...
		for _, s := range i.IPSubnet {
			_, subnet, err := net.ParseCIDR(s.Prefix)
			if err != nil {
				logger.Error("Failed to parse subnet", zap.String("prefix", s.Prefix), zap.Error(err))
				continue
			}

			ap, err := local.newAddressPool(ifName, priority, subnet)
			if err != nil {
				logger.Error("Failed to create pool", zap.Any("subnet", subnet), zap.String("ifName", ifName), zap.Error(err))
				continue
			}

			addressCount := 0
			// For each address in the subnet...
			for _, a := range s.IPAddress {
				// Primary addresses are reserved for the host.
				if a.IsPrimary {
					continue
				}

				address := net.ParseIP(a.Address)

				_, err = ap.newAddressRecord(&address)
				if err != nil {
					logger.Error("Failed to create", zap.Any("address", address), zap.Error(err))
					continue
				}
				addressCount++
			}
			logger.Info("got addresses from interface subnet", zap.Int("addressCount", addressCount), zap.String("ifName", ifName), zap.Any("subnet", subnet))
		}
	}

	// Set the local address space as active.
	return s.sink.setAddressSpace(local)
}
