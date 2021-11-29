package policies

import "github.com/Azure/azure-container-networking/npm/util"

// returns two booleans indicating whether the network policy has ingress and egress respectively
func (networkPolicy *NPMNetworkPolicy) hasIngressAndEgress() (hasIngress, hasEgress bool) {
	hasIngress = false
	hasEgress = false
	for _, aclPolicy := range networkPolicy.ACLs {
		hasIngress = hasIngress || aclPolicy.hasIngress()
		hasEgress = hasEgress || aclPolicy.hasEgress()
	}
	return
}

func (networkPolicy *NPMNetworkPolicy) egressChainName() string {
	return networkPolicy.chainName(util.IptablesAzureEgressPolicyChainPrefix)
}

func (networkPolicy *NPMNetworkPolicy) ingressChainName() string {
	return networkPolicy.chainName(util.IptablesAzureIngressPolicyChainPrefix)
}

func (networkPolicy *NPMNetworkPolicy) chainName(prefix string) string {
	policyHash := util.Hash(networkPolicy.PolicyKey)
	return joinWithDash(prefix, policyHash)
}
