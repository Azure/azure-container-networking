package dataplane

import (
	"encoding/json"
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

	// used for lints
	hcnSchemaMajorVersion = 2
	hcnSchemaMinorVersion = 0
)

var errPolicyModeUnsupported = errors.New("only IPSet policy mode is supported")

// initializeDataPlane will help gather network and endpoint details
func (dp *DataPlane) initializeDataPlane() error {
	klog.Infof("[DataPlane] Initializing dataplane for windows")

	if dp.PolicyMode == "" {
		dp.PolicyMode = policies.IPSetPolicyMode
	}
	if dp.PolicyMode != policies.IPSetPolicyMode {
		return errPolicyModeUnsupported
	}

	err := dp.getNetworkInfo()
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to get network info", err)
	}

	// Initialize Endpoint query used to filter healthy endpoints (vNIC) of Windows pods
	dp.endpointQuery.query = hcn.HostComputeQuery{
		SchemaVersion: hcn.SchemaVersion{
			Major: hcnSchemaMajorVersion,
			Minor: hcnSchemaMinorVersion,
		},
		Flags: hcn.HostComputeQueryFlagsNone,
	}
	// Filter out any endpoints that are not in "AttachedShared" State. All running Windows pods with networking must be in this state.
	filterMap := map[string]uint16{"State": hcnEndpointStateAttachedSharing}
	filter, err := json.Marshal(filterMap)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to marshal endpoint filter map", err)
	}
	dp.endpointQuery.query.Filter = string(filter)

	// reset endpoint cache so that netpol references are removed for all endpoints while refreshing pod endpoints
	// no need to lock endpointCache at boot up
	dp.endpointCache.cache = make(map[string]*npmEndpoint)

	return nil
}

func (dp *DataPlane) getNetworkInfo() error {
	retryNumber := 0
	ticker := time.NewTicker(time.Second * time.Duration(maxNoNetSleepTime))
	defer ticker.Stop()

	var err error
	for ; true; <-ticker.C {
		err = dp.setNetworkIDByName(dp.NetworkName)
		if err == nil || !isNetworkNotFoundErr(err) {
			return err
		}
		retryNumber++
		if retryNumber >= maxNoNetRetryCount {
			break
		}
		klog.Infof("[DataPlane Windows] Network with name %s not found. Retrying in %d seconds, Current retry number %d, max retries: %d",
			dp.NetworkName,
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
		return npmerrors.SimpleErrorWrapper("failed to initialize dataplane", err)
	}

	// for backwards compatibility, get remote allEndpoints to delete as well
	allEndpoints, err := dp.getAllPodEndpoints()
	if err != nil {
		return err
	}

	// TODO once we make endpoint refreshing smarter, it would be most efficient to use allEndpoints to refreshPodEndpoints here.
	// But currently, we call refreshPodEndpoints for every Pod event, so this optimization wouldn't do anything for now.
	// There's also no need to refreshPodEndpoints at bootup since we don't know of any Pods at this point, and the endpoint cache is only needed for known Pods.

	epIDs := make([]string, len(allEndpoints))
	for k, e := range allEndpoints {
		epIDs[k] = e.Id
	}

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
// 2. Will check for existing applicable network policies and applies it on endpoint.
// Assumption: a Pod won't take up its previously used IP when restarting (see https://stackoverflow.com/questions/52362514/when-will-the-kubernetes-pod-ip-change)
func (dp *DataPlane) updatePod(pod *updateNPMPod) error {
	klog.Infof("[DataPlane] updatePod called. podKey: %s", pod.PodKey)
	if len(pod.IPSetsToAdd) == 0 && len(pod.IPSetsToRemove) == 0 {
		// nothing to do
		return nil
	}

	// lock the endpoint cache while we read/modify the endpoint with the pod's IP
	dp.endpointCache.Lock()
	defer dp.endpointCache.Unlock()

	// Check if pod is already present in cache
	endpoint, ok := dp.endpointCache.cache[pod.PodIP]
	if !ok {
		// ignore this err and pod endpoint will be deleted in ApplyDP
		// if the endpoint is not found, it means the pod is not part of this node or pod got deleted.
		klog.Warningf("[DataPlane] ignoring pod update since there is no corresponding endpoint. IP: %s. podKey: %s", pod.PodIP, pod.PodKey)
		return nil
	}

	if endpoint.podKey == unspecifiedPodKey {
		// while refreshing pod endpoints, newly discovered endpoints are given an unspecified pod key
		klog.Infof("[DataPlane] associating pod with endpoint. podKey: %s. endpoint: %+v", pod.PodKey, endpoint)
		endpoint.podKey = pod.PodKey
	} else if pod.PodKey == endpoint.previousIncorrectPodKey {
		klog.Infof("[DataPlane] ignoring pod update since this pod was previously and incorrectly assigned to this endpoint. endpoint: %+v", endpoint)
		return nil
	} else if pod.PodKey != endpoint.podKey {
		// solves issue 1729
		klog.Infof("[DataPlane] pod key has changed. will reset endpoint acls and skip looking ipsets to remove. new podKey: %s. previous endpoint: %+v", pod.PodKey, endpoint)
		if err := dp.policyMgr.ResetEndpoint(endpoint.id); err != nil {
			return fmt.Errorf("failed to reset endpoint for pod with incorrect pod key. new podKey: %s. previous endpoint: %+v. err: %w", pod.PodKey, endpoint, err)
		}

		// mark this after successful reset. If before reset, we would not retry on failure
		endpoint.previousIncorrectPodKey = endpoint.podKey
		endpoint.podKey = pod.PodKey

		// all ACLs were removed, so in case there were ipsets to remove, there's no need to look for policies to delete
		pod.IPSetsToRemove = nil

		if dp.NetworkName == util.CalicoNetworkName {
			klog.Infof("adding back base ACLs for calico CNI endpoint after resetting ACLs. endpoint: %+v", endpoint)
			dp.policyMgr.AddBaseACLsForCalicoCNI(endpoint.id)
		}
	}

	// for every ipset we're removing from the endpoint, remove from the endpoint any policy that requires the set
	for _, setName := range pod.IPSetsToRemove {
		/*
			Scenarios:
			1. There's a chance a policy is/was just removed, but the ipset's selector hasn't been updated yet.
			   We may try to remove the policy again here, which is ok.

			2. If a policy is added to the ipset's selector after getting the selector (meaning dp.AddPolicy() was called),
			   we won't try to remove the policy, which is fine since the policy must've never existed on the endpoint.

			3. If a policy is added to the ipset's selector in a dp.AddPolicy() thread AFTER getting the selector here,
			   then the ensuing policyMgr.AddPolicy() call will wait for this function to release the endpointCache lock.

			4. If a policy is added to the ipset's selector in a dp.AddPolicy() thread BEFORE getting the selector here,
			   there could be a race between policyMgr.RemovePolicy() here and policyMgr.AddPolicy() there.
		*/
		selectorReference, err := dp.ipsetMgr.GetSelectorReferencesBySet(setName)
		if err != nil {
			// ignore this set since it may have been deleted in the background reconcile thread
			klog.Infof("[DataPlane] ignoring pod update for ipset to remove since the set does not exist. pod: %+v. set: %s", pod, setName)
			continue
		}

		for policyKey := range selectorReference {
			// Now check if any of these network policies are applied on this endpoint.
			// If yes then proceed to delete the network policy.
			if _, ok := endpoint.netPolReference[policyKey]; ok {
				// Delete the network policy
				endpointList := map[string]string{
					endpoint.ip: endpoint.id,
				}
				err := dp.policyMgr.RemovePolicyForEndpoints(policyKey, endpointList)
				if err != nil {
					return err
				}
				delete(endpoint.netPolReference, policyKey)
			}
		}
	}

	// for every ipset we're adding to the endpoint, consider adding to the endpoint every policy that the set touches
	// add policy if:
	// 1. it's not already there
	// 2. the pod IP is part of every set that the policy requires (every set in the pod selector)
	toAddPolicies := make(map[string]struct{})
	for _, setName := range pod.IPSetsToAdd {
		/*
			Scenarios:
			1. If a policy is added to the ipset's selector after getting the selector (meaning dp.AddPolicy() was called),
			   we will miss adding the policy here, but will add the policy to all endpoints in that other thread, which has
			   to wait on the endpointCache lock when calling getEndpointsToApplyPolicy().

			2. We may add the policy here and in the dp.AddPolicy() thread if the policy is added to the ipset's selector before
			   that other thread calls policyMgr.AddPolicy(), which is ok.

			3. FIXME: If a policy is/was just removed, but the ipset's selector hasn't been updated yet,
			   we may try to add the policy again here...
		*/
		selectorReference, err := dp.ipsetMgr.GetSelectorReferencesBySet(setName)
		if err != nil {
			// ignore this set since it may have been deleted in the background reconcile thread
			klog.Infof("[DataPlane] ignoring pod update for ipset to remove since the set does not exist. pod: %+v. set: %s", pod, setName)
			continue
		}

		for policyKey := range selectorReference {
			if _, ok := endpoint.netPolReference[policyKey]; ok {
				continue
			}

			policy, ok := dp.policyMgr.GetPolicy(policyKey)
			if !ok {
				klog.Infof("[DataPlane] while updating pod, policy is referenced but does not exist. pod: [%s], policy: [%s], set [%s]", pod.PodKey, policyKey, setName)
				continue
			}

			selectorIPSets := dp.getSelectorIPSets(policy)
			ok, err := dp.ipsetMgr.DoesIPSatisfySelectorIPSets(pod.PodIP, pod.PodKey, selectorIPSets)
			if err != nil {
				return fmt.Errorf("[DataPlane] error getting IPs satisfying selector ipsets: %w", err)
			}
			if !ok {
				continue
			}

			toAddPolicies[policyKey] = struct{}{}
		}
	}

	if len(toAddPolicies) == 0 {
		return nil
	}

	successfulPolicies, err := dp.policyMgr.AddAllPolicies(toAddPolicies, endpoint.id, endpoint.ip)
	for policyKey := range successfulPolicies {
		endpoint.netPolReference[policyKey] = struct{}{}
	}
	if err != nil {
		return fmt.Errorf("failed to add all policies while updating pod. endpoint: %+v. policies: %+v. err: %w", endpoint, toAddPolicies, err)
	}

	klog.Infof("[DataPlane] updatedPod complete. podKey: %s. endpoint: %+v", pod.PodKey, endpoint)

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
	selectorIPSets := dp.getSelectorIPSets(policy)
	netpolSelectorIPs, err := dp.ipsetMgr.GetIPsFromSelectorIPSets(selectorIPSets)
	if err != nil {
		return nil, err
	}

	// lock the endpoint cache while we read/modify the endpoints with IPs in the policy's pod selector
	dp.endpointCache.Lock()
	defer dp.endpointCache.Unlock()

	endpointList := make(map[string]string)
	for ip, podKey := range netpolSelectorIPs {
		endpoint, ok := dp.endpointCache.cache[ip]
		if !ok {
			klog.Infof("[DataPlane] ignoring selector IP since it was not found in the endpoint cache and might not be in the HNS network. ip: %s. podKey: %s", ip, podKey)
			continue
		}

		if endpoint.podKey != podKey {
			// in case the pod controller hasn't updated the dp yet that the IP's pod owner has changed
			klog.Infof("[DataPlane] ignoring selector IP since the endpoint is assigned to a different podKey. ip: %s. podKey: %s. endpoint: %+v", ip, podKey, endpoint)
			continue
		}

		endpointList[ip] = endpoint.id
		endpoint.netPolReference[policy.PolicyKey] = struct{}{}
	}
	return endpointList, nil
}

func (dp *DataPlane) getAllPodEndpoints() ([]*hcn.HostComputeEndpoint, error) {
	klog.Infof("getting all endpoints for network ID %s", dp.networkID)
	endpoints, err := dp.ioShim.Hns.ListEndpointsOfNetwork(dp.networkID)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to get all pod endpoints", err)
	}

	epPointers := make([]*hcn.HostComputeEndpoint, 0, len(endpoints))
	for k := range endpoints {
		epPointers = append(epPointers, &endpoints[k])
	}
	return epPointers, nil
}

func (dp *DataPlane) getLocalPodEndpoints() ([]*hcn.HostComputeEndpoint, error) {
	klog.Info("getting local endpoints")
	endpoints, err := dp.ioShim.Hns.ListEndpointsQuery(dp.endpointQuery.query)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to get local pod endpoints", err)
	}

	epPointers := make([]*hcn.HostComputeEndpoint, 0, len(endpoints))
	for k := range endpoints {
		epPointers = append(epPointers, &endpoints[k])
	}
	return epPointers, nil
}

// refreshPodEndpoints will refresh all the pod endpoints and create empty netpol references for new endpoints
/*
Key Assumption: a new pod event (w/ IP) cannot come before HNS knows (and can tell us) about the endpoint.
From NPM logs, it seems that endpoints are updated far earlier (several seconds) before the pod event comes in.

What we learn from refreshing endpoints:
- an old endpoint doesn't exist anymore
- a new endpoint has come up

Why not refresh when adding a netpol to all required pods?
- It's ok if we try to apply on an endpoint that doesn't exist anymore.
- We won't know the pod associated with a new endpoint even if we refresh.

Why can we refresh only once before updating all pods in the updatePodCache (see ApplyDataplane)?
- Again, it's ok if we try to apply on a non-existent endpoint.
- We won't miss the endpoint (see the assumption). At the time the pod event came in (when AddToSets/RemoveFromSets were called), HNS already knew about the endpoint.
*/
func (dp *DataPlane) refreshPodEndpoints() error {
	endpoints, err := dp.getLocalPodEndpoints()
	if err != nil {
		return err
	}

	// lock the endpoint cache while we reconcile with HNS goal state
	dp.endpointCache.Lock()
	defer dp.endpointCache.Unlock()

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

		oldNPMEP, ok := dp.endpointCache.cache[ip]
		if !ok {
			// add the endpoint to the cache if it's not already there
			npmEP := newNPMEndpoint(endpoint)
			dp.endpointCache.cache[ip] = npmEP
			// NOTE: TSGs rely on this log line
			klog.Infof("updating endpoint cache to include %s: %+v", npmEP.ip, npmEP)

			if dp.NetworkName == util.CalicoNetworkName {
				// NOTE 1: connectivity may be broken for an endpoint until this method is called
				// NOTE 2: if NPM restarted, technically we could call into HNS to add the base ACLs even if they already exist on the Endpoint.
				// It doesn't seem worthwhile to account for these edge-cases since using calico network is currently intended just for testing
				klog.Infof("adding base ACLs for calico CNI endpoint. IP: %s. ID: %s", ip, npmEP.id)
				dp.policyMgr.AddBaseACLsForCalicoCNI(npmEP.id)
			}
		} else if oldNPMEP.id != endpoint.Id {
			// multiple endpoints can have the same IP address, but there should be one endpoint ID per pod
			// throw away old endpoints that have the same IP as a current endpoint (the old endpoint is getting deleted)
			// we don't have to worry about cleaning up network policies on endpoints that are getting deleted
			npmEP := newNPMEndpoint(endpoint)
			klog.Infof("[DataPlane] updating endpoint cache for IP with a new endpoint. old endpoint: %+v. new endpoint: %+v", oldNPMEP, npmEP)
			dp.endpointCache.cache[ip] = npmEP

			if dp.NetworkName == util.CalicoNetworkName {
				// NOTE 1: connectivity may be broken for an endpoint until this method is called
				// NOTE 2: if NPM restarted, technically we could call into HNS to add the base ACLs even if they already exist on the Endpoint.
				// It doesn't seem worthwhile to account for these edge-cases since using calico network is currently intended just for testing
				klog.Infof("adding base ACLs for calico CNI endpoint. IP: %s. ID: %s", ip, npmEP.id)
				dp.policyMgr.AddBaseACLsForCalicoCNI(npmEP.id)
			}
		}
	}

	// garbage collection for the endpoint cache
	for ip, ep := range dp.endpointCache.cache {
		if _, ok := existingIPs[ip]; !ok {
			klog.Infof("[DataPlane] deleting endpoint from cache. endpoint: %+v", ep)
			delete(dp.endpointCache.cache, ip)
		}
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

func isNetworkNotFoundErr(err error) bool {
	return strings.Contains(err.Error(), fmt.Sprintf("Network name %q not found", util.AzureNetworkName)) ||
		strings.Contains(err.Error(), fmt.Sprintf("Network name %q not found", util.CalicoNetworkName))
}
