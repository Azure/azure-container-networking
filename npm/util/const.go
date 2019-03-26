// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

//kubernetes related constants.
const (
	KubeSystemFlag          string = "kube-system"
	KubePodTemplateHashFlag string = "pod-template-hash"
	KubeAllPodsFlag         string = "all-pod"
	KubeAllNamespacesFlag   string = "all-namespace"
	KubeAppFlag             string = "k8s-app"
	KubeProxyFlag           string = "kube-proxy"

	// The version of k8s that accept "AND" between namespaceSelector and podSelector is "1.11"
	k8sMajorVerForNewPolicyDef string = "1"
	k8sMinorVerForNewPolicyDef string = "11"
)

//iptables related constants.
const (
	Iptables                         string = "iptables"
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
	IptablesRelatedState             string = "RELATED"
	IptablesEstablishedState         string = "ESTABLISHED"
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
