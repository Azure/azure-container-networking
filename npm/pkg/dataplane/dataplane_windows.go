package dataplane

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog"
)

const (
	policyWithSets policyMode = "policyWithSets"
	policyWithIPs  policyMode = "policyWithIPs"
)

func (dp *DataPlane) setPolicyMode() {
	dp.policyMode = policyWithSets
	err := hcn.SetPolicySupported()
	if err != nil {
		dp.policyMode = policyWithIPs
	}
}

// initializeDataPlane will help gather network and endpoint details
func (dp *DataPlane) initializeDataPlane() error {
	klog.Infof("[DataPlane] Initializing dataplane for windows")
	// policy mode is only needed for windows, move this to a more central position.
	if dp.policyMode == "" {
		dp.setPolicyMode()
	}

	err := dp.setNetworkIDByName(AzureNetworkName)
	if err != nil {
		return err
	}

	err = dp.refreshAllPodEndpoints()
	if err != nil {
		return err
	}

	return nil
}

func (dp *DataPlane) shouldUpdatePod() bool {
	return true
}

// updatePod has two responsibilities in windows
// 1. Will call into dataplane and updates endpoint references of this pod.
// 2. Will check for existing applicable network policies and applies it on endpoint
func (dp *DataPlane) updatePod(pod *updateNPMPod) error {
	klog.Infof("[DataPlane] updatePod called for Pod Key %s", pod.PodKey)
	// Check if pod is part of this node
	if pod.NodeName != dp.nodeName {
		klog.Infof("[DataPlane] ignoring update pod as expected Node: [%s] got: [%s]", dp.nodeName, pod.NodeName)
		return nil
	}

	err := dp.refreshAllPodEndpoints()
	if err != nil {
		klog.Infof("[DataPlane] failed to refresh endpoints in updatePod with %s", err.Error())
		return err
	}

	// Check if pod is already present in cache
	endpoint, ok := dp.endpointCache[pod.PodIP]
	if !ok {
		return fmt.Errorf("[DataPlane] did not find endpoint with IPaddress %s", pod.PodIP)
	}

	if endpoint.IP != pod.PodIP {
		// If the existing endpoint ID has changed, it means that the Pod has been recreated
		// this results in old endpoint to be deleted, so we can safely ignore cleaning up policies
		// and delete it from the cache.
		delete(dp.endpointCache, pod.PodIP)
	}
	// Check if the removed IPSets have any network policy references
	for _, setName := range pod.IPSetsToRemove {
		selectorReference, err := dp.ipsetMgr.GetSelectorReferencesBySet(setName)
		if err != nil {
			return err
		}

		for policyName := range selectorReference {
			// Now check if any of these network policies are applied on this endpoint.
			// If yes then proceed to delete the network policy
			// Remove policy should be deleting this netpol reference
			if _, ok := endpoint.NetPolReference[policyName]; ok {
				// Delete the network policy
				endpointList := map[string]string{
					endpoint.IP: endpoint.ID,
				}
				err := dp.policyMgr.RemovePolicy(policyName, endpointList)
				if err != nil {
					return err
				}
				delete(endpoint.NetPolReference, policyName)
			}
		}
	}

	// Check if any of the existing network policies needs to be applied
	toAddPolicies := make(map[string]struct{})
	for _, setName := range pod.IPSetsToAdd {
		selectorReference, err := dp.ipsetMgr.GetSelectorReferencesBySet(setName)
		if err != nil {
			return err
		}

		for netpol := range selectorReference {
			toAddPolicies[netpol] = struct{}{}
		}
	}

	// Now check if any of these network policies are applied on this endpoint.
	// If not then proceed to apply the network policy
	for policyName := range toAddPolicies {
		if _, ok := endpoint.NetPolReference[policyName]; ok {
			continue
		}
		// TODO Also check if the endpoint reference in policy for this Ip is right
		netpolSelectorIPs, err := dp.getSelectorIPsByPolicyName(policyName)
		if err != nil {
			return err
		}

		if _, ok := netpolSelectorIPs[pod.PodIP]; !ok {
			continue
		}

		// Apply the network policy
		policy, ok := dp.policyMgr.GetPolicy(policyName)
		if !ok {
			return fmt.Errorf("policy with name %s does not exist", policyName)
		}

		endpointList := map[string]string{
			endpoint.IP: endpoint.ID,
		}
		err = dp.policyMgr.AddPolicy(policy, endpointList)
		if err != nil {
			return err
		}

		endpoint.NetPolReference[policyName] = struct{}{}
	}

	return nil
}

func (dp *DataPlane) getSelectorIPsByPolicyName(policyName string) (map[string]struct{}, error) {
	policy, ok := dp.policyMgr.GetPolicy(policyName)
	if !ok {
		return nil, fmt.Errorf("policy with name %s does not exist", policyName)
	}

	return dp.getSelectorIPsByPolicy(policy)
}

func (dp *DataPlane) getSelectorIPsByPolicy(policy *policies.NPMNetworkPolicy) (map[string]struct{}, error) {
	selectorIpSets := make(map[string]struct{})
	for _, ipset := range policy.PodSelectorIPSets {
		selectorIpSets[ipset.Metadata.GetPrefixName()] = struct{}{}
	}

	return dp.ipsetMgr.GetIPsFromSelectorIPSets(selectorIpSets)
}

func (dp *DataPlane) getEndpointsToApplyPolicy(policy *policies.NPMNetworkPolicy) (map[string]string, error) {
	err := dp.refreshAllPodEndpoints()
	if err != nil {
		klog.Infof("[DataPlane] failed to refresh endpoints in getEndpointsToApplyPolicy with %s", err.Error())
		return nil, err
	}

	// TODO need to calculate all existing selector
	netpolSelectorIPs, err := dp.getSelectorIPsByPolicy(policy)
	if err != nil {
		return nil, err
	}

	endpointList := make(map[string]string)
	for ip := range netpolSelectorIPs {
		endpoint, ok := dp.endpointCache[ip]
		if !ok {
			// this endpoint might not be in this particular Node.
			klog.Infof("[DataPlane] Ignoring endpoint with IP %s. Not found in endpointCache", ip)
			continue
		}
		endpointList[ip] = endpoint.ID
		// TODO make sure this is netpol key and not name
		endpoint.NetPolReference[policy.Name] = struct{}{}
	}
	return endpointList, nil
}

func (dp *DataPlane) resetDataPlane() error {
	return nil
}

func (dp *DataPlane) getAllPodEndpoints() ([]hcn.HostComputeEndpoint, error) {
	klog.Infof("Getting all endpoints for Network ID %s", dp.networkID)
	endpoints, err := dp.ioShim.Hns.ListEndpointsOfNetwork(dp.networkID)
	if err != nil {
		return nil, err
	}
	return endpoints, nil
}

// refreshAllPodEndpoints will refresh all the pod endpoints
func (dp *DataPlane) refreshAllPodEndpoints() error {
	endpoints, err := dp.getAllPodEndpoints()
	if err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		klog.Infof("Endpoints info %+v", endpoint.Id)
		if len(endpoint.IpConfigurations) == 0 {
			klog.Infof("Endpoint ID %s has no IPAddreses", endpoint.Id)
			continue
		}
		ip := endpoint.IpConfigurations[0].IpAddress
		if ip == "" {
			klog.Infof("Endpoint ID %s has empty IPAddress field", endpoint.Id)
			continue
		}
		ep := &NPMEndpoint{
			Name:            endpoint.Name,
			ID:              endpoint.Id,
			NetPolReference: make(map[string]struct{}),
			IP:              endpoint.IpConfigurations[0].IpAddress,
		}

		dp.endpointCache[ep.IP] = ep
	}
	return nil
}

func (dp *DataPlane) setNetworkIDByName(networkName string) error {
	// Get Network ID
	network, err := dp.ioShim.Hns.GetNetworkByName(networkName)
	if err != nil {
		return err
	}

	dp.networkID = network.Id
	return nil
}

func (dp *DataPlane) getEndpointByIP(podIP string) (*NPMEndpoint, error) {
	endpoints, err := dp.getAllPodEndpoints()
	if err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		for _, ipConfig := range endpoint.IpConfigurations {
			if ipConfig.IpAddress == podIP {
				ep := &NPMEndpoint{
					Name:            endpoint.Name,
					ID:              endpoint.Id,
					IP:              endpoint.IpConfigurations[0].IpAddress,
					NetPolReference: make(map[string]struct{}),
				}
				return ep, nil
			}
		}
	}

	return nil, nil
}
