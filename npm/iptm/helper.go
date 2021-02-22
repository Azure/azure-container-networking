package iptm

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/util"
)

// GetAllChainsAndRules returns all NPM chains and rules
func getAllChainsAndRules() [][]string {
	funcList := []func() [][]string{
		getAzureNPMChainRules,
		getAzureNPMIngressPortChainRules,
		getAzureNPMIngressFromChainRules,
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
			util.IptablesAzureIngressPortChain,
		},
		{
			util.IptablesAzureChain,
			util.IptablesJumpFlag,
			util.IptablesAzureEgressPortChain,
		},
		{
			util.IptablesAzureChain,
			util.IptablesJumpFlag,
			util.IptablesAccept,
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
			util.IptablesAccept,
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
			util.IptablesAccept,
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
			util.IptablesJumpFlag,
			util.IptablesAzureTargetSetsChain,
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
