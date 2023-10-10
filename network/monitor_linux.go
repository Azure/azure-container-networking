package network

import (
	"fmt"

	cnms "github.com/Azure/azure-container-networking/cnms/cnmspackage"
	"github.com/Azure/azure-container-networking/ebtables"
	"go.uber.org/zap"
)

const (
	ipv6Mask = "/ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"
)

// monitorNetworkState compares current ebtable nat rules with state rules and matches state.
func (nm *networkManager) monitorNetworkState(networkMonitor *cnms.NetworkMonitor) error {
	currentEbtableRulesMap, err := cnms.GetEbTableRulesInMap()
	if err != nil {
		logger.Error("GetEbTableRulesInMap failed with", zap.Error(err))
		return err
	}

	currentStateRulesMap := nm.AddStateRulesToMap()
	networkMonitor.CreateRequiredL2Rules(currentEbtableRulesMap, currentStateRulesMap)
	networkMonitor.RemoveInvalidL2Rules(currentEbtableRulesMap, currentStateRulesMap)

	return nil
}

// AddStateRulesToMap adds rules to state based off network manager settings.
func (nm *networkManager) AddStateRulesToMap() map[string]string {
	rulesMap := make(map[string]string)

	for _, extIf := range nm.ExternalInterfaces {
		arpDnatKey := fmt.Sprintf("-p ARP -i %s --arp-op Reply -j dnat --to-dst ff:ff:ff:ff:ff:ff --dnat-target ACCEPT", extIf.Name)
		rulesMap[arpDnatKey] = ebtables.PreRouting

		snatKey := fmt.Sprintf("-s Unicast -o %s -j snat --to-src %s --snat-arp --snat-target ACCEPT", extIf.Name, extIf.MacAddress.String())
		rulesMap[snatKey] = ebtables.PostRouting

		for _, extIP := range extIf.IPAddresses {
			if extIP.IP.To4() != nil {
				arpReplyKey := fmt.Sprintf("-p ARP --arp-op Request --arp-ip-dst %s -j arpreply --arpreply-mac %s", extIP.IP.String(), extIf.MacAddress.String())
				rulesMap[arpReplyKey] = ebtables.PreRouting
			}
		}

		for _, nw := range extIf.Networks {
			for _, ep := range nw.Endpoints {
				for _, ipAddr := range ep.IPAddresses {
					if ipAddr.IP.To4() != nil {
						arpReplyKey := fmt.Sprintf("-p ARP --arp-op Request --arp-ip-dst %s -j arpreply --arpreply-mac %s", ipAddr.IP.String(), ep.MacAddress.String())
						rulesMap[arpReplyKey] = ebtables.PreRouting
					}

					dst := "--ip-dst"
					proto := "IPv4"
					ipAddress := ipAddr.IP.String()
					if ipAddr.IP.To4() == nil {
						dst = "--ip6-dst"
						proto = "IPv6"
						ipAddress = ipAddr.IP.String() + ipv6Mask
					}

					dnatMacKey := fmt.Sprintf("-p %s -i %s %s %s -j dnat --to-dst %s --dnat-target ACCEPT",
						proto, extIf.Name, dst, ipAddress, ep.MacAddress.String())
					rulesMap[dnatMacKey] = ebtables.PreRouting
				}
			}
		}
	}

	return rulesMap
}
