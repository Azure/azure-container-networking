package iptm

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/util"
)

// getAllDefaultRules returns all NPM chains and rules
func getAllDefaultRules() [][]string {
	funcList := []func() [][]string{
		getAzureNPMChainRules,
		getAzureNPMAcceptChainRules,
		getAzureNPMIngressChainRules,
		getAzureNPMIngressPortChainRules,
		getAzureNPMEgressChainRules,
		getAzureNPMEgressPortChainRules,
		getAzureNPMIngressDropsChainRules,
		getAzureNPMEgressDropsChainRules,
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
		{
			util.IptablesAzureIngressPortChain,
			util.IptablesJumpFlag,
			util.IptablesAzureIngressFromChain,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("ALL-JUMP-TO-%s", util.IptablesAzureIngressFromChain),
		},
	}
}

// getAzureNPMIngressDropsChainRules returns rules for AZURE-NPM-INGRESS-DROPS
func getAzureNPMIngressDropsChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureIngressDropsChain,
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

// getAzureNPMEgressPortChainRules returns rules for AZURE-NPM-EGRESS-PORT
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
		{
			util.IptablesAzureEgressPortChain,
			util.IptablesJumpFlag,
			util.IptablesAzureEgressToChain,
			util.IptablesModuleFlag,
			util.IptablesCommentModuleFlag,
			util.IptablesCommentFlag,
			fmt.Sprintf("ALL-JUMP-TO-%s", util.IptablesAzureEgressToChain),
		},
	}
}

// getAzureNPMEgressDropsChainRules returns rules for AZURE-NPM-EGRESS-DROPS
func getAzureNPMEgressDropsChainRules() [][]string {
	return [][]string{
		{
			util.IptablesAzureEgressDropsChain,
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
			util.IptablesAzureEgressDropsChain,
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
