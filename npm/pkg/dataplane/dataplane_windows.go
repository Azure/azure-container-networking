package dataplane

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog"
)

// initializeDataPlane will help gather network and endpoint details
func (dp *DataPlane) initializeDataPlane() error {
	klog.Infof("[DataPlane] Initializing dataplane for windows")

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
func (dp *DataPlane) updatePod(pod *UpdateNPMPod) error {
	klog.Infof("[DataPlane] updatePod called for %s/%s", pod.Namespace, pod.Name)
	// Check if pod is part of this node
	if pod.NodeName != dp.nodeName {
		klog.Infof("[DataPlane] ignoring update pod as expected Node: [%s] got: [%s]", dp.nodeName, pod.NodeName)
		return nil
	}

	podKey := getNPMPodKey(pod.Namespace, pod.Name)
	// Check if pod is already present in cache
	endpoint, ok := dp.endpointCache[podKey]
	if (!ok) || (endpoint.IP != pod.PodIP) {
		// If the existing endpoint ID has changed, it means that the Pod has been recreated
		// this results in old endpoint to be deleted, so we can safely ignore cleaning up policies
		// Get endpoint for this pod
		endpoint, err := dp.getEndpointByIP(pod.PodIP)
		if err != nil {
			return err
		}
		dp.endpointCache[podKey] = endpoint
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
				err := dp.policyMgr.RemovePolicy(policyName, []string{endpoint.ID})
				if err != nil {
					return err
				}
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
		err = dp.policyMgr.AddPolicy(policy, []string{endpoint.ID})
		if err != nil {
			return err
		}
	}

	return nil
}

func (dp *DataPlane) getSelectorIPsByPolicyName(policyName string) (map[string]struct{}, error) {
	policy, ok := dp.policyMgr.GetPolicy(policyName)
	if !ok {
		return nil, fmt.Errorf("policy with name %s does not exist", policyName)
	}

	var selectorIpSets map[string]struct{}
	for ipsetName := range policy.PodSelectorIPSets {
		selectorIpSets[ipsetName] = struct{}{}
	}

	return dp.ipsetMgr.GetIPsFromSelectorIPSets(selectorIpSets)
}

func (dp *DataPlane) getEndpointsToApplyPolicy(policy *policies.NPMNetworkPolicy) (map[string]string, error) {
	// TODO need to calculate all existing selector
	return nil, nil
}

func (dp *DataPlane) resetDataPlane() error {
	return nil
}

func (dp *DataPlane) getAllPodEndpoints() ([]hcn.HostComputeEndpoint, error) {
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
		klog.Infof("Endpoints info %+v", endpoint.Policies)
		ep := &NPMEndpoint{
			Name:            endpoint.Name,
			ID:              endpoint.Id,
			NetPolReference: make(map[string]struct{}),
		}

		dp.endpointCache[ep.Name] = ep
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

func getNPMPodKey(nameSpace, name string) string {
	return nameSpace + "/" + name
}
