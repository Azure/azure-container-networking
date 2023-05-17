// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/common"
)

// kubernetes related constants.
const (
	KubeSystemFlag             string = "kube-system"
	KubePodTemplateHashFlag    string = "pod-template-hash"
	KubeAllPodsFlag            string = "all-pod"
	KubeAllNamespacesFlag      string = "all-namespaces"
	KubeAppFlag                string = "k8s-app"
	KubeProxyFlag              string = "kube-proxy"
	KubePodStatusFailedFlag    string = "Failed"
	KubePodStatusSucceededFlag string = "Succeeded"
	KubePodStatusUnknownFlag   string = "Unknown"

	// The version of k8s that accept "AND" between namespaceSelector and podSelector is "1.11"
	k8sMajorVerForNewPolicyDef string = "1"
	k8sMinorVerForNewPolicyDef string = "11"
)

var (
	Iptables        = IptablesLegacy
	Ip6tables       = Ip6tablesLegacy //nolint (avoid warning to capitalize this p)
	IptablesSave    = IptablesSaveLegacy
	IptablesRestore = IptablesRestoreLegacy
)

// iptables related constants.
const (
	PlaceAzureChainAfterKubeServices = false
	PlaceAzureChainFirst             = true

	IptablesNft                string = "iptables-nft"
	Ip6tablesLegacy            string = "ip6tables" //nolint (avoid warning to capitalize this p)
	IptablesSaveNft            string = "iptables-nft-save"
	IptablesRestoreNft         string = "iptables-nft-restore"
	IptablesLegacy             string = "iptables"
	IptablesSaveLegacy         string = "iptables-save"
	IptablesRestoreLegacy      string = "iptables-restore"
	IptablesRestoreNoFlushFlag string = "--noflush"
	IptablesRestoreTableFlag   string = "-T"
	IptablesRestoreCommit      string = "COMMIT"
	IptablesConfigFile         string = "/var/log/iptables.conf"
	IptablesTestConfigFile     string = "/var/log/iptables-test.conf"
	IptablesLockFile           string = "/run/xtables.lock"
	IptablesChainCreationFlag  string = "-N"
	IptablesInsertionFlag      string = "-I"
	IptablesAppendFlag         string = "-A"
	IptablesDeletionFlag       string = "-D"
	IptablesFlushFlag          string = "-F"
	IptablesCheckFlag          string = "-C"
	IptablesDestroyFlag        string = "-X"
	IptablesJumpFlag           string = "-j"
	IptablesWaitFlag           string = "-w"
	IptablesDefaultWaitTime    string = "60"
	IptablesAccept             string = "ACCEPT"
	IptablesReject             string = "REJECT"
	IptablesDrop               string = "DROP"
	IptablesReturn             string = "RETURN"
	IptablesMark               string = "MARK"
	IptablesSrcFlag            string = "src"
	IptablesDstFlag            string = "dst"
	IptablesNamedPortFlag      string = "dst,dst"
	IptablesNotFlag            string = "!"
	IptablesProtFlag           string = "-p"
	IptablesSFlag              string = "-s"
	IptablesDFlag              string = "-d"
	IptablesDstPortFlag        string = "--dport"
	IptablesSrcPortFlag        string = "--sport"
	IptablesModuleFlag         string = "-m"
	IptablesSetModuleFlag      string = "set"
	IptablesMatchSetFlag       string = "--match-set"
	IptablesSetMarkFlag        string = "--set-mark"
	IptablesMarkFlag           string = "--mark"
	IptablesMarkVerb           string = "mark"
	IptablesStateModuleFlag    string = "state"
	IptablesStateFlag          string = "--state"
	IptablesCtstateModuleFlag  string = "conntrack" // state module is obsolete: https://unix.stackexchange.com/questions/108169/what-is-the-difference-between-m-conntrack-ctstate-and-m-state-state
	IptablesCtstateFlag        string = "--ctstate"
	IptablesMultiportFlag      string = "multiport"
	IptablesRelatedState       string = "RELATED"
	IptablesEstablishedState   string = "ESTABLISHED"
	IptablesNewState           string = "NEW"
	IptablesFilterTable        string = "filter"
	IptablesCommentModuleFlag  string = "comment"
	IptablesCommentFlag        string = "--comment"
	IptablesAddCommentFlag

	IptablesTableFlag       string = "-t"
	IptablesListFlag        string = "-L"
	IptablesNumericFlag     string = "-n"
	IptablesLineNumbersFlag string = "--line-numbers"

	IptablesKubeServicesChain          string = "KUBE-SERVICES"
	IptablesForwardChain               string = "FORWARD"
	IptablesInputChain                 string = "INPUT"
	IptablesAzureChain                 string = "AZURE-NPM"
	IptablesAzureAcceptChain           string = "AZURE-NPM-ACCEPT"
	IptablesAzureKubeSystemChain       string = "AZURE-NPM-KUBE-SYSTEM"
	IptablesAzureIngressChain          string = "AZURE-NPM-INGRESS"
	IptablesAzureIngressAllowMarkChain string = "AZURE-NPM-INGRESS-ALLOW-MARK"
	IptablesAzureEgressChain           string = "AZURE-NPM-EGRESS"

	// Chains used in NPM v1
	IptablesAzureIngressPortChain  string = "AZURE-NPM-INGRESS-PORT"
	IptablesAzureIngressFromChain  string = "AZURE-NPM-INGRESS-FROM"
	IptablesAzureEgressPortChain   string = "AZURE-NPM-EGRESS-PORT"
	IptablesAzureEgressToChain     string = "AZURE-NPM-EGRESS-TO"
	IptablesAzureIngressDropsChain string = "AZURE-NPM-INGRESS-DROPS"
	IptablesAzureEgressDropsChain  string = "AZURE-NPM-EGRESS-DROPS"

	// NPM v2 Chains
	IptablesAzureIngressPolicyChainPrefix string = "AZURE-NPM-INGRESS"
	IptablesAzureEgressPolicyChainPrefix  string = "AZURE-NPM-EGRESS"

	// Below chain exists only in NPM before v1.2.6
	IptablesAzureTargetSetsChain string = "AZURE-NPM-TARGET-SETS"
	// Below chain existing only in NPM before v1.2.7
	IptablesAzureIngressWrongDropsChain string = "AZURE-NPM-INRGESS-DROPS"
	// Below chains exists only for before Azure-NPM:v1.0.27
	// and should be removed after a baking period.
	IptablesAzureIngressFromNsChain  string = "AZURE-NPM-INGRESS-FROM-NS"
	IptablesAzureIngressFromPodChain string = "AZURE-NPM-INGRESS-FROM-POD"
	IptablesAzureEgressToNsChain     string = "AZURE-NPM-EGRESS-TO-NS"
	IptablesAzureEgressToPodChain    string = "AZURE-NPM-EGRESS-TO-POD"

	// Below are the skb->mark NPM will use for different criteria
	// for V1
	IptablesAzureClearMarkHex string = "0x0"
	// for v2, deprecated
	IptablesAzureClearMarkHexV2 string = "0x0/0xE00"

	// marks in NPM v2
	// NPM uses the 3rd word of the 32-bit mark for the purpose of
	// identifying the traffic direction and decision making.
	// NPM uses 9th, 10th and 11th bit for marking
	IptablesAzureIngressAllowMarkHex string = "0x200/0x200"
	IptablesAzureIngressDropMarkHex  string = "0x400/0x400"
	IptablesAzureEgressDropMarkHex   string = "0x800/0x800"

	// marks in NPM v1
	IptablesAzureIngressMarkHex string = "0x2000"
	// IptablesAzureEgressXMarkHex is used for us to not override but append to the existing MARK
	// https://unix.stackexchange.com/a/283455 comment contains the explanation on
	// MARK manipulations with offset.
	IptablesAzureEgressXMarkHex string = "0x1000/0x1000"
	// IptablesAzureEgressMarkHex is for checking the absolute value of the mark
	IptablesAzureEgressMarkHex string = "0x1000"
	IptablesAzureAcceptMarkHex string = "0x3000"
)

// ipset related constants.
const (
	Ipset               string = "ipset"
	IpsetSaveFlag       string = "save"
	IpsetRestoreFlag    string = "restore"
	IpsetConfigFile     string = "/var/log/ipset.conf"
	IpsetTestConfigFile string = "/var/log/ipset-test.conf"
	IpsetCreationFlag   string = "-N"
	IpsetAppendFlag     string = "-A"
	IpsetDeletionFlag   string = "-D"
	IpsetFlushFlag      string = "-F"
	IpsetDestroyFlag    string = "-X"

	IpsetExistFlag     string = "-exist"
	IpsetFileFlag      string = "-file"
	IPsetCheckListFlag string = "list"
	IpsetTestFlag      string = "test"

	IpsetSetGenericFlag string = "setgeneric" // not used in ipset commands, used as an internal identifier for nethash/hash:ip,port
	IpsetSetListFlag    string = "setlist"
	IpsetNetHashFlag    string = "nethash"
	IpsetIPPortHashFlag string = "hash:ip,port"

	IpsetUDPFlag  string = "udp:"
	IpsetSCTPFlag string = "sctp:"
	IpsetTCPFlag  string = "tcp:"

	IpsetLabelDelimter string = ":"

	AzureNpmFlag   string = "azure-npm"
	AzureNpmPrefix string = "azure-npm-"

	IpsetMaxelemName string = "maxelem" // todo, what's using this?
	IpsetMaxelemNum  string = "4294967295"

	IpsetNomatch string = "nomatch"

	// Prefixes for ipsets
	NamedPortIPSetPrefix string = "namedport:"
	NamespacePrefix      string = "ns-"
	NamespaceLabelPrefix string = "nslabel-"
	PodLabelPrefix       string = "podlabel-"
	CIDRPrefix           string = "cidr-"
	NestedLabelPrefix    string = "nestedlabel-"
	EmptySetPrefix       string = "empty-"

	NegationPrefix string = "not-"

	SetPolicyDelimiter string = ","
)

const (
	BashCommand     string = "bash"
	BashCommandFlag string = "-c"
)

// NPM telemetry constants.
const (
	AddNamespaceEvent    string = "Add Namespace"
	UpdateNamespaceEvent string = "Update Namespace"
	DeleteNamespaceEvent string = "Delete Namespace"

	AddPodEvent    string = "Add Pod"
	UpdatePodEvent string = "Update Pod"
	DeletePodEvent string = "Delete Pod"

	AddNetworkPolicyEvent    string = "Add network policy"
	UpdateNetworkPolicyEvent string = "Update network policy"
	DeleteNetworkPolicyEvent string = "Delete network policy"

	ErrorMetric  string = "ErrorMetric"
	PackageName  string = "PackageName"
	FunctionName string = "FunctionName"
	ErrorCode    string = "ErrorCode"

	// Default batch size in AI telemetry
	// Defined here https://docs.microsoft.com/en-us/azure/azure-monitor/app/pricing
	BatchSizeInBytes          int = 32768
	BatchIntervalInSecs       int = 30
	RefreshTimeoutInSecs      int = 15
	GetEnvRetryCount          int = 5
	GetEnvRetryWaitTimeInSecs int = 3
	AiInitializeRetryCount    int = 3
	AiInitializeRetryInMin    int = 1

	DebugMode bool = true

	ErrorValue float64 = 1
)

const (
	// AzureNetworkName is the default network Azure CNI creates
	AzureNetworkName = "azure"
	// CalicoNetworkName is the default network Calico CNI creates
	CalicoNetworkName = "Calico"
)

// These ID represents where did the error log generate from.
// It's for better query purpose. In Kusto these value are used in
// OperationID column
const (
	NpmID  int = iota + 1
	IpsmID     // ipsetmanager in v2
	IptmID     // policymanager in v2
	NSID
	PodID
	NetpolID
	UtilID
	DaemonID
	ControllerID
	DaemonDataplaneID // for v2
	FanOutServerID    // for v2
)

func DetectIptablesVersion(ioShim *common.IOShim) {
	cmd := ioShim.Exec.Command(IptablesSaveNft, "-t", "mangle")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Error running iptables-nft-save: %s", err)
		return
	}

	if strings.Contains(string(output), "KUBE-IPTABLES-HINT") || strings.Contains(string(output), "KUBE-KUBELET-CANARY") {
		Iptables = IptablesNft
		IptablesSave = IptablesSaveNft
		IptablesRestore = IptablesRestoreNft
	} else {
		lCmd := ioShim.Exec.Command(IptablesSaveLegacy, "-t", "mangle")

		loutput, err := lCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error running iptables-legacy-save: %s", err)
			return
		}

		if strings.Contains(string(loutput), "KUBE-IPTABLES-HINT") || strings.Contains(string(loutput), "KUBE-KUBELET-CANARY") {
			Iptables = IptablesLegacy
			IptablesSave = IptablesSaveLegacy
			IptablesRestore = IptablesRestoreLegacy
		} else {
			lsavecmd := ioShim.Exec.Command(IptablesSaveNft)
			lsaveoutput, err := lsavecmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Error running iptables-nft-save: %s", err)
				return
			}

			lcount := countLines(lsaveoutput)

			savecmd := ioShim.Exec.Command(IptablesSaveLegacy)
			saveoutput, err := savecmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Error running iptables-legacy-save: %s", err)
				return
			}

			count := countLines(saveoutput)

			if lcount > count {
				Iptables = IptablesLegacy
				IptablesSave = IptablesSaveLegacy
				IptablesRestore = IptablesRestoreLegacy
			} else {
				Iptables = IptablesNft
				IptablesSave = IptablesSaveNft
				IptablesRestore = IptablesRestoreNft
			}
		}
	}
}

func countLines(output []byte) int {
	count := 0
	for _, x := range bytes.Split(output, []byte("\n")) {
		if len(x) >= 1 && x[0] == '-' {
			count++
		}
	}
	return count
}
