package dataplane

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/Microsoft/hcsshim/hcn"
	"k8s.io/klog"
)

const (
	maxNoNetRetryCount int = 240 // max wait time 240*5 == 20 mins
	maxNoNetSleepTime  int = 5   // in seconds
)

var (
	errPolicyModeUnsupported = errors.New("only IPSet policy mode is supported")
	errMismanagedPodKey      = errors.New("the pod key was not managed correctly when refreshing pod endpoints")
)

// initializeDataPlane will help gather network and endpoint details
func (dp *DataPlane) initializeDataPlane() error {
	klog.Infof("[DataPlane] Initializing dataplane for windows")

	if dp.PolicyMode == "" {
		dp.PolicyMode = policies.IPSetPolicyMode
	}
	if dp.PolicyMode != policies.IPSetPolicyMode {
		return errPolicyModeUnsupported
	}
	if err := hcn.SetPolicySupported(); err != nil {
		return npmerrors.SimpleErrorWrapper("[DataPlane] kernel does not support SetPolicies", err)
	}

	err := dp.getNetworkInfo()
	if err != nil {
		return err
	}

	// reset endpoint cache so that netpol references are removed for all endpoints while refreshing pod endpoints
	dp.endpointCache = make(map[string]*npmEndpoint)
	err = dp.refreshAllPodEndpoints()
	if err != nil {
		return err
	}

	return nil
}

func (dp *DataPlane) getNetworkInfo() error {
	retryNumber := 0
	ticker := time.NewTicker(time.Second * time.Duration(maxNoNetSleepTime))
	defer ticker.Stop()

	var err error
	for ; true; <-ticker.C {
		err = dp.setNetworkIDByName(util.AzureNetworkName)
		if err == nil || !isNetworkNotFoundErr(err) {
			return err
		}
		retryNumber++
		if retryNumber >= maxNoNetRetryCount {
			break
		}
		klog.Infof("[DataPlane Windows] Network with name %s not found. Retrying in %d seconds, Current retry number %d, max retries: %d",
			util.AzureNetworkName,
			maxNoNetSleepTime,
			retryNumber,
			maxNoNetRetryCount,
		)
	}

	return fmt.Errorf("failed to get network info after %d retries with err %w", maxNoNetRetryCount, err)
}

func (dp *DataPlane) bootupDataPlane() error {
	// initialize the DP so the podendpoints will get updated.
	if err := dp.initializeDataPlane(); err != nil {
		return err
	}

	epIDs := dp.getAllEndpointIDs()

	// It is important to keep order to clean-up ACLs before ipsets. Otherwise we won't be able to delete ipsets referenced by ACLs
	if err := dp.policyMgr.Bootup(epIDs); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupDataplane, false, "failed to reset policy dataplane", err)
	}
	if err := dp.ipsetMgr.ResetIPSets(); err != nil {
		return npmerrors.ErrorWrapper(npmerrors.BootupDataplane, false, "failed to reset ipsets dataplane", err)
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
		// ignore this err and pod endpoint will be deleted in ApplyDP
		// if the endpoint is not found, it means the pod is not part of this node or pod got deleted.
		klog.Warningf("[DataPlane] did not find endpoint with IPaddress %s", pod.PodIP)
		return nil
	}

	if endpoint.podKey == unspecifiedPodKey {
		// while refreshing pod endpoints, newly discovered endpoints are given an unspecified pod key
		if endpoint.isStalePodKey(pod.PodKey) {
			// NOTE: if a pod restarts and takes up its previous IP, then its endpoint would be new and this branch would be taken.
			// Updates to this pod would not occur. Pod IPs are expected to change on restart though.
			// See: https://stackoverflow.com/questions/52362514/when-will-the-kubernetes-pod-ip-change
			// If a pod does restart and take up its previous IP, then the pod can be deleted/restarted to mitigate this problem.
			klog.Infof("ignoring pod update since pod with key %s is stale and likely was deleted", pod.PodKey)
			return nil
		}
		endpoint.podKey = pod.PodKey
	} else if pod.PodKey != endpoint.podKey {
		return fmt.Errorf("pod key mismatch. Expected: %s, Actual: %s. Error: [%w]", pod.PodKey, endpoint.podKey, errMismanagedPodKey)
	}

	// for every ipset we're removing from the endpoint, remove from the endpoint any policy that requires the set
	for _, setName := range pod.IPSetsToRemove {
		selectorReference, err := dp.ipsetMgr.GetSelectorReferencesBySet(setName)
		if err != nil {
			return err
		}

		for policyKey := range selectorReference {
			// Now check if any of these network policies are applied on this endpoint.
			// If yes then proceed to delete the network policy
			// Remove policy should be deleting this netpol reference
			if _, ok := endpoint.netPolReference[policyKey]; ok {
				// Delete the network policy
				endpointList := map[string]string{
					endpoint.ip: endpoint.id,
				}
				err := dp.policyMgr.RemovePolicy(policyKey, endpointList)
				if err != nil {
					return err
				}
				delete(endpoint.netPolReference, policyKey)
			}
		}
	}

	// for every ipset we're adding to the endpoint, consider adding to the endpoint every policy that the set touches
	toAddPolicies := make(map[string]struct{})
	for _, setName := range pod.IPSetsToAdd {
		selectorReference, err := dp.ipsetMgr.GetSelectorReferencesBySet(setName)
		if err != nil {
			return err
		}

		for policyKey := range selectorReference {
			if _, ok := dp.pendingPolicies[policyKey]; !ok {
				toAddPolicies[policyKey] = struct{}{}
			}
		}
	}

	// for all of these policies, add the policy to the endpoint if:
	// 1. it's not already there
	// 2. the pod IP is part of every set that the policy requires (every set in the pod selector)
	for policyKey := range toAddPolicies {
		if _, ok := endpoint.netPolReference[policyKey]; ok {
			continue
		}

		// TODO Also check if the endpoint reference in policy for this Ip is right
		policy, ok := dp.policyMgr.GetPolicy(policyKey)
		if !ok {
			return fmt.Errorf("policy with name %s does not exist", policyKey)
		}

		selectorIPSets := dp.getSelectorIPSets(policy)
		ok, err := dp.ipsetMgr.DoesIPSatisfySelectorIPSets(pod.PodIP, selectorIPSets)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		// Apply the network policy
		endpointList := map[string]string{
			endpoint.ip: endpoint.id,
		}
		err = dp.policyMgr.AddPolicy(policy, endpointList)
		if err != nil {
			return err
		}

		endpoint.netPolReference[policyKey] = struct{}{}
	}

	return nil
}

func (dp *DataPlane) getSelectorIPSets(policy *policies.NPMNetworkPolicy) map[string]struct{} {
	selectorIpSets := make(map[string]struct{})
	for _, ipset := range policy.PodSelectorIPSets {
		selectorIpSets[ipset.Metadata.GetPrefixName()] = struct{}{}
	}
	klog.Infof("policy %s has policy selector: %+v", policy.PolicyKey, selectorIpSets)
	return selectorIpSets
}

func (dp *DataPlane) getEndpointsToApplyPolicy(policy *policies.NPMNetworkPolicy) (map[string]string, error) {
	err := dp.refreshAllPodEndpoints()
	if err != nil {
		klog.Infof("[DataPlane] failed to refresh endpoints in getEndpointsToApplyPolicy with %s", err.Error())
		return nil, err
	}

	selectorIPSets := dp.getSelectorIPSets(policy)
	netpolSelectorIPs, err := dp.ipsetMgr.GetIPsFromSelectorIPSets(selectorIPSets)
	if err != nil {
		return nil, err
	}

	endpointList := make(map[string]string)
	for ip := range netpolSelectorIPs {
		endpoint, ok := dp.endpointCache[ip]
		if !ok {
			klog.Infof("[DataPlane] Ignoring endpoint with IP %s since it was not found in the endpoint cache. This IP might not be in the HNS network", ip)
			continue
		}
		endpointList[ip] = endpoint.id
		endpoint.netPolReference[policy.PolicyKey] = struct{}{}
	}
	return endpointList, nil
}

func (dp *DataPlane) getAllPodEndpoints() ([]hcn.HostComputeEndpoint, error) {
	klog.Infof("Getting all endpoints for Network ID %s", dp.networkID)
	endpoints, err := dp.ioShim.Hns.ListEndpointsOfNetwork(dp.networkID)
	if err != nil {
		return nil, err
	}
	return endpoints, nil
}

// refreshAllPodEndpoints will refresh all the pod endpoints and create empty netpol references for new endpoints
func (dp *DataPlane) refreshAllPodEndpoints() error {
	endpoints, err := dp.getAllPodEndpoints()
	if err != nil {
		return err
	}

	currentTime := time.Now().Unix()
	existingIPs := make(map[string]struct{})
	for _, endpoint := range endpoints {
		if len(endpoint.IpConfigurations) == 0 {
			klog.Infof("Endpoint ID %s has no IPAddreses", endpoint.Id)
			continue
		}
		ip := endpoint.IpConfigurations[0].IpAddress
		if ip == "" {
			klog.Infof("Endpoint ID %s has empty IPAddress field", endpoint.Id)
			continue
		}

		existingIPs[ip] = struct{}{}

		oldNPMEP, ok := dp.endpointCache[ip]
		if !ok {
			// add the endpoint to the cache if it's not already there
			npmEP := newNPMEndpoint(&endpoint)
			dp.endpointCache[ip] = npmEP
			// NOTE: TSGs rely on this log line
			klog.Infof("updating endpoint cache to include %s: %+v", npmEP.ip, npmEP)
		} else if oldNPMEP.id != endpoint.Id {
			// multiple endpoints can have the same IP address, but there should be one endpoint ID per pod
			// throw away old endpoints that have the same IP as a current endpoint (the old endpoint is getting deleted)
			// we don't have to worry about cleaning up network policies on endpoints that are getting deleted
			npmEP := newNPMEndpoint(&endpoint)
			if oldNPMEP.podKey == unspecifiedPodKey {
				klog.Infof("updating endpoint cache since endpoint changed for IP which never had a pod key. new endpoint: %s, old endpoint: %s, ip: %s", npmEP.id, oldNPMEP.id, npmEP.ip)
				dp.endpointCache[ip] = npmEP
			} else {
				npmEP.stalePodKey = &staleKey{
					key:       oldNPMEP.podKey,
					timestamp: currentTime,
				}
				dp.endpointCache[ip] = npmEP
				// NOTE: TSGs rely on this log line
				klog.Infof("updating endpoint cache for previously cached IP %s: %+v with stalePodKey %+v", npmEP.ip, npmEP, npmEP.stalePodKey)
			}
		}
	}

	// garbage collection for the endpoint cache
	for ip, ep := range dp.endpointCache {
		if _, ok := existingIPs[ip]; !ok {
			if ep.podKey == unspecifiedPodKey {
				if ep.stalePodKey == nil {
					klog.Infof("deleting old endpoint which never had a pod key. ID: %s, IP: %s", ep.id, ip)
					delete(dp.endpointCache, ip)
				} else if int(currentTime-ep.stalePodKey.timestamp)/60 > minutesToKeepStalePodKey {
					klog.Infof("deleting old endpoint which had a stale pod key. ID: %s, IP: %s, stalePodKey: %+v", ep.id, ip, ep.stalePodKey)
					delete(dp.endpointCache, ip)
				}
			} else {
				ep.stalePodKey = &staleKey{
					key:       ep.podKey,
					timestamp: currentTime,
				}
				ep.podKey = unspecifiedPodKey
				klog.Infof("marking endpoint stale for at least %d minutes. ID: %s, IP: %s, new stalePodKey: %+v", minutesToKeepStalePodKey, ep.id, ip, ep.stalePodKey)
			}
		}
	}

	klog.Infof("Endpoint cache after refresh: %+v", dp.endpointCache) // TODO: remove for public preview

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

func (dp *DataPlane) getAllEndpointIDs() []string {
	endpointIDs := make([]string, 0, len(dp.endpointCache))
	for _, endpoint := range dp.endpointCache {
		endpointIDs = append(endpointIDs, endpoint.id)
	}
	return endpointIDs
}

func isNetworkNotFoundErr(err error) bool {
	return strings.Contains(err.Error(), fmt.Sprintf("Network name \"%s\" not found", util.AzureNetworkName))
}
