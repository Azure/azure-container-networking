// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

//kubernetes related constants.
const (
	KubeSystemFlag             string = "kube-system"
	KubePodTemplateHashFlag    string = "pod-template-hash"
	KubeAllPodsFlag            string = "all-pod"
	KubeAllNamespacesFlag      string = "all-namespace"
	KubeAppFlag                string = "k8s-app"
	KubeProxyFlag              string = "kube-proxy"
	KubePodStatusFailedFlag    string = "Failed"
	KubePodStatusSucceededFlag string = "Succeeded"
	KubePodStatusUnknownFlag   string = "Unknown"

	// The version of k8s that accept "AND" between namespaceSelector and podSelector is "1.11"
	k8sMajorVerForNewPolicyDef string = "1"
	k8sMinorVerForNewPolicyDef string = "11"
)

//iptables related constants.
const (
	Iptables                         string = "iptables"
	Ip6tables                        string = "ip6tables"
	IptablesSave                     string = "iptables-save"
	IptablesRestore                  string = "iptables-restore"
	IptablesConfigFile               string = "/var/log/iptables.conf"
	IptablesTestConfigFile           string = "/var/log/iptables-test.conf"
	IptablesLockFile                 string = "/run/xtables.lock"
	IptablesChainCreationFlag        string = "-N"
	IptablesInsertionFlag            string = "-I"
	IptablesAppendFlag               string = "-A"
	IptablesDeletionFlag             string = "-D"
	IptablesFlushFlag                string = "-F"
	IptablesCheckFlag                string = "-C"
	IptablesDestroyFlag              string = "-X"
	IptablesJumpFlag                 string = "-j"
	IptablesWaitFlag                 string = "-w"
	IptablesAccept                   string = "ACCEPT"
	IptablesReject                   string = "REJECT"
	IptablesDrop                     string = "DROP"
	IptablesSrcFlag                  string = "src"
	IptablesDstFlag                  string = "dst"
	IptablesProtFlag                 string = "-p"
	IptablesSFlag                    string = "-s"
	IptablesDFlag                    string = "-d"
	IptablesDstPortFlag              string = "--dport"
	IptablesMatchFlag                string = "-m"
	IptablesSetFlag                  string = "set"
	IptablesMatchSetFlag             string = "--match-set"
	IptablesStateFlag                string = "state"
	IptablesMatchStateFlag           string = "--state"
	IptablesMultiportFlag            string = "multiport"
	IptablesMultiDestportFlag        string = "--dports"
	IptablesRelatedState             string = "RELATED"
	IptablesEstablishedState         string = "ESTABLISHED"
	IptablesFilterTable              string = "filter"
	IptablesAzureChain               string = "AZURE-NPM"
	IptablesAzureIngressPortChain    string = "AZURE-NPM-INGRESS-PORT"
	IptablesAzureIngressFromChain    string = "AZURE-NPM-INGRESS-FROM"
	IptablesAzureIngressFromNsChain  string = "AZURE-NPM-INGRESS-FROM-NS"
	IptablesAzureIngressFromPodChain string = "AZURE-NPM-INGRESS-FROM-POD"
	IptablesAzureEgressPortChain     string = "AZURE-NPM-EGRESS-PORT"
	IptablesAzureEgressToChain       string = "AZURE-NPM-EGRESS-TO"
	IptablesAzureEgressToNsChain     string = "AZURE-NPM-EGRESS-TO-NS"
	IptablesAzureEgressToPodChain    string = "AZURE-NPM-EGRESS-TO-POD"
	IptablesAzureTargetSetsChain     string = "AZURE-NPM-TARGET-SETS"
	IptablesForwardChain             string = "FORWARD"
	IptablesInputChain               string = "INPUT"
)

//ipset related constants.
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

	IpsetExistFlag string = "-exist"
	IpsetFileFlag  string = "-file"

	IpsetSetListFlag string = "setlist"
	IpsetNetHashFlag string = "nethash"

	AzureNpmFlag   string = "azure-npm"
	AzureNpmPrefix string = "azure-npm-"
)

//NPM telemetry constants.
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
)

//VFP related constants.
const (
	VFPTagFlag         string = "tag"
	VFPNLTagFlag       string = "nltag"
	TagConfigFile      string = "C:\\k\\tag.conf"
	TagTestConfigFile  string = "C:\\k\\tag-test.conf"
	RuleConfigFile     string = "C:\\k\\rule.conf"
	RuleTestConfigFile string = "C:\\k\\rule-test.conf"
	VFPCmd             string = "vfpctrl.exe"
	VFPError           string = "ERROR"
	// Port related
	Port         string = "/port"
	ListPortCmd  string = "/list-vmswitch-port"
	PortSplit    string = "Port name"
	PortFriendly string = "Port Friendly name"
	MACAddress   string = "MAC address"
	GUIDLength   int    = 36
	// Layer related
	Layer            string = "/layer"
	ListLayerCmd     string = "/list-layer"
	AddLayerCmd      string = "/add-layer"
	NPMLayer         string = "NPM_LAYER"
	StatefulLayer    string = "stateful"
	StatelessLayer   string = "stateless"
	NPMLayerPriority string = "10"
	RemoveLayerCmd   string = "/remove-layer"
	// Group related
	Group                     string = "/group"
	AddGroupCmd               string = "/add-group"
	NPMIngressGroup           string = "NPM_INGRESS"
	NPMIngressPriority        string = "100"
	NPMIngressDefaultGroup    string = "NPM_INGRESS_DEFAULT"
	NPMIngressDefaultPriority string = "101"
	NPMEgressGroup            string = "NPM_EGRESS"
	NPMEgressPriority         string = "102"
	NPMEgressDefaultGroup     string = "NPM_EGRESS_DEFAULT"
	NPMEgressDefaultPriority  string = "103"
	DirectionIn               string = "in"
	DirectionOut              string = "out"
	RemoveGroupCmd            string = "/remove-group"
	GroupLabel                string = "GROUP : "
	// Tag related
	Tag             string = "/tag"
	ListTagCmd      string = "/list-tag"
	AddTagCmd       string = "/add-tag"
	IPV4            string = "ipv4"
	IPV6            string = "ipv6"
	ReplaceTagCmd   string = "/replace-tag"
	RemoveTagCmd    string = "/remove-tag"
	RemoveAllTagCmd string = "/remove-all-tag"
	TagLabel        string = "TAG : "
	TagIPLabel      string = "Tag IP : "
	// Rule related
	Rule             string = "/rule"
	ListRuleCmd      string = "/list-rule"
	AddRuleCmd       string = "/add-rule"
	AddTagRuleCmd    string = "/add-tag-rule"
	RemoveRuleCmd    string = "/remove-rule"
	RemoveAllRuleCmd string = "/remove-all-rule"
	RuleLabel        string = "RULE : "
	Allow            string = "allow"
	Block            string = "block"
)