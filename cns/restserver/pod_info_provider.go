// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package restserver

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/pkg/errors"
)

// EndpointStatePodInfoByIP projects CNS-owned infra endpoint state by IP.
func EndpointStatePodInfoByIP(state map[string]*EndpointInfo) (map[string]cns.PodInfo, error) {
	podInfoByIP := make(map[string]cns.PodInfo)
	for containerID, endpointInfo := range state {
		for _, ipInfo := range endpointInfo.IfnameToIPMap {
			if !ipInfo.NICType.IsInfraOrLegacy() {
				continue
			}
			addIP := func(ipConfig net.IPNet) error {
				ip := ipConfig.IP.String()
				if _, ok := podInfoByIP[ip]; ok {
					return errors.Wrap(cns.ErrDuplicateIP, ip)
				}
				podInfoByIP[ip] = cns.NewPodInfo(
					containerID,
					containerID,
					endpointInfo.PodName,
					endpointInfo.PodNamespace,
				)
				return nil
			}
			for _, ipConfig := range ipInfo.IPv4 {
				if err := addIP(ipConfig); err != nil {
					return nil, err
				}
			}
			for _, ipConfig := range ipInfo.IPv6 {
				if err := addIP(ipConfig); err != nil {
					return nil, err
				}
			}
		}
	}
	return podInfoByIP, nil
}

// UnifiedPodInfoByIPProvider reads pod information from the active unified cache.
func (service *HTTPRestService) UnifiedPodInfoByIPProvider() cns.PodInfoByIPProvider {
	return cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
		if service.selectedUnifiedStateAdapter() == nil {
			return nil, fmt.Errorf("unified endpoint state provider is not active")
		}
		service.RLock()
		defer service.RUnlock()
		return EndpointStatePodInfoByIP(service.EndpointState)
	})
}
