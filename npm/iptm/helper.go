package iptm

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/util"
)

// GetAllChainsAndRules returns all NPM chains and rules
func getAllChainsAndRules() [][]string {
	funcList := []func() [][]string{
		getAzureNPMChainRules,
		getAzureNPMAcceptChainRules,
		getAzureNPMIngressChainRules,
		getAzureNPMIngressPortChainRules,
		getAzureNPMIngressFromChainRules,
		getAzureNPMEgressChainRules,
		getAzureNPMEgressPortChainRules,
		getAzureNPMEgressToChainRules,
	}

	chainsAndRules := [][]string{}
	for _, fn := range funcList {
		tempRules := fn()
		chainsAndRules = append(chainsAndRules, tempRules...)
	}

	return chainsAndRules
}

// getAzureNPMChainRules returns all rules for AZURE-NPM chain
func getAzureNPMChainRules() [][]string {
	// Note: make sure 0th index is prent chain for logging
	return [][]string{
		{
			util.IptablesAzureChain,
			util.IptablesJumpFlag,
			util.IptablesAzureIngressChain,
		},
		{
			util.IptablesAzureChain,
			util.IptablesJumpFlag,
			util.IptablesAzureEgressChain,
		},
		{
			util.IptablesAzureChain,
			util.IptablesJumpFlag,
			util.IptablesAzureAcceptChain,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureAcceptMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("ACCEPT-on-INGRESS-and-EGRESS-mark-%s", util.IptablesAzureAcceptMarkHex),
		},
		{
			util.IptablesAzureChain,
			util.IptablesJumpFlag,
			util.IptablesAzureAcceptChain,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureIngressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("ACCEPT-on-INGRESS-mark-%s", util.IptablesAzureIngressMarkHex),
		},
		{
			util.IptablesAzureChain,
			util.IptablesJumpFlag,
			util.IptablesAzureAcceptChain,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureEgressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("ACCEPT-on-EGRESS-mark-%s", util.IptablesAzureEgressMarkHex),
		},
		{
			util.IptablesAzureChain,
			util.IptablesModuleFlag,
			util.IptablesStateModuleFlag,
			util.IptablesStateFlag,
			util.IptablesRelatedState + "," + util.IptablesEstablishedState,
			util.IptablesJumpFlag,
			util.IptablesAccept,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("ACCEPT-on-connection-state"),
		},
	}
}

// getAzureNPMAcceptChainRules clears all marks and accepts packets
func getAzureNPMAcceptChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureAcceptChain,
			util.IptablesJumpFlag,
			util.IptablesMark,
			util.IptablesSetMarkFlag,
			util.IptablesAzureClearMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("Clear-AZURE-NPM-MARKS"),
		},
		{
			util.IptablesAzureAcceptChain,
			util.IptablesJumpFlag,
			util.IptablesAccept,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("ACCEPT-All-packets"),
		},
	}
}

// getAzureNPMIngressChainRules returns rules for AZURE-NPM-INGRESS-PORT
func getAzureNPMIngressChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureIngressChain,
			util.IptablesJumpFlag,
			util.IptablesAzureIngressPortChain,
		},
		{
			util.IptablesAzureIngressChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureIngressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-INGRESS-mark-%s", util.IptablesAzureIngressMarkHex),
		},
		{
			util.IptablesAzureIngressChain,
			util.IptablesJumpFlag,
			util.IptablesAzureIngressDropsChain,
		},
	}
}

// getAzureNPMIngressPortChainRules returns rules for AZURE-NPM-INGRESS-PORT
func getAzureNPMIngressPortChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureIngressPortChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureIngressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-INGRESS-mark-%s", util.IptablesAzureIngressMarkHex),
		},
	}
}

// getAzureNPMIngressFromChainRules returns rules for AZURE-NPM-INGRESS-PORT
func getAzureNPMIngressFromChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureIngressFromChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureIngressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-INGRESS-mark-%s", util.IptablesAzureIngressMarkHex),
		},
	}
}

// getAzureNPMEgressChainRules returns rules for AZURE-NPM-INGRESS-PORT
func getAzureNPMEgressChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureEgressChain,
			util.IptablesJumpFlag,
			util.IptablesAzureEgressPortChain,
		},
		{
			util.IptablesAzureEgressChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureAcceptMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-EGRESS-and-INGRESS-mark-%s", util.IptablesAzureAcceptMarkHex),
		},
		{
			util.IptablesAzureEgressChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureEgressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-EGRESS-mark-%s", util.IptablesAzureEgressMarkHex),
		},
		{
			util.IptablesAzureEgressChain,
			util.IptablesJumpFlag,
			util.IptablesAzureEgressDropsChain,
		},
	}
}

// getAzureNPMEgressPortChainRules returns rules for AZURE-NPM-INGRESS-PORT
func getAzureNPMEgressPortChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureEgressPortChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureAcceptMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-EGRESS-and-INGRESS-mark-%s", util.IptablesAzureAcceptMarkHex),
		},
		{
			util.IptablesAzureEgressPortChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureEgressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-EGRESS-mark-%s", util.IptablesAzureEgressMarkHex),
		},
	}
}

// getAzureNPMEgressToChainRules returns rules for AZURE-NPM-INGRESS-PORT
func getAzureNPMEgressToChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureEgressToChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureAcceptMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-EGRESS-and-INGRESS-mark-%s", util.IptablesAzureAcceptMarkHex),
		},
		{
			util.IptablesAzureEgressToChain,
			util.IptablesJumpFlag,
			util.IptablesReturn,
			util.IptablesModuleFlag,
			util.IptablesMarkVerb,
			util.IptablesMarkFlag,
			util.IptablesAzureEgressMarkHex,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("RETURN-on-EGRESS-mark-%s", util.IptablesAzureEgressMarkHex),
		},
	}
}
