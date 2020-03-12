package network

import (
	"fmt"

	cnms "github.com/Azure/azure-container-networking/cnms/cnmspackage"
	"github.com/Azure/azure-container-networking/ebtables"
	"github.com/Azure/azure-container-networking/log"
)

func (nm *networkManager) monitorNetworkState(networkMonitor *cnms.NetworkMonitor) error {

	currentEbtableRulesMap, err := cnms.GetEbTableRulesInMap()

	if err != nil {
		log.Printf("GetEbTableRulesInMap failed with error %v", err)
		return err
	}

	currentStateRulesMap := nm.AddStateRulesToMap()

	networkMonitor.CreateRequiredL2Rules(currentEbtableRulesMap, currentStateRulesMap)
	networkMonitor.RemoveInvalidL2Rules(currentEbtableRulesMap, currentStateRulesMap)

	return nil
}

func (nm *networkManager) AddStateRulesToMap() map[string]string {

	rulesMap := make(map[string]string)

	for _, extIf := range nm.ExternalInterfaces {
		arpDnatKey := fmt.Sprintf("-p ARP -i %s --arp-op Reply -j dnat --to-dst ff:ff:ff:ff:ff:ff --dnat-target ACCEPT", extIf.Name)
		rulesMap[arpDnatKey] = ebtables.PreRouting

		snatKey := fmt.Sprintf("-s Unicast -o %s -j snat --to-src %s --snat-arp --snat-target ACCEPT", extIf.Name, extIf.MacAddress.String())
		rulesMap[snatKey] = ebtables.PostRouting

		for _, nw := range extIf.Networks {
			for _, ep := range nw.Endpoints {
				for _, ipAddr := range ep.IPAddresses {
					arpReplyKey := fmt.Sprintf("-p ARP --arp-op Request --arp-ip-dst %s -j arpreply --arpreply-mac %s", ipAddr.IP.String(), ep.MacAddress.String())
					rulesMap[arpReplyKey] = ebtables.PreRouting

					dnatMacKey := fmt.Sprintf("-p IPv4 -i %s --ip-dst %s -j dnat --to-dst %s --dnat-target ACCEPT", extIf.Name, ipAddr.IP.String(), ep.MacAddress.String())
					rulesMap[dnatMacKey] = ebtables.PreRouting
				}
			}
		}
	}

	return rulesMap
}
