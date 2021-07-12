// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package util

//kubernetes related constants.
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
	CommitBytes = []byte("COMMIT")
	SpaceBytes  = []byte(" ")
)

//iptables related constants.
const (
	Iptables                  string = "iptables"
	Ip6tables                 string = "ip6tables"
	IptablesSave              string = "iptables-save"
	IptablesRestore           string = "iptables-restore"
	IptablesConfigFile        string = "/var/log/iptables.conf"
	IptablesTestConfigFile    string = "/var/log/iptables-test.conf"
	IptablesLockFile          string = "/run/xtables.lock"
	IptablesChainCreationFlag string = "-N"
	IptablesInsertionFlag     string = "-I"
	IptablesAppendFlag        string = "-A"
	IptablesDeletionFlag      string = "-D"
	IptablesFlushFlag         string = "-F"
	IptablesCheckFlag         string = "-C"
	IptablesDestroyFlag       string = "-X"
	IptablesJumpFlag          string = "-j"
	IptablesWaitFlag          string = "-w"
	IptablesAccept            string = "ACCEPT"
	IptablesReject            string = "REJECT"
	IptablesDrop              string = "DROP"
	IptablesReturn            string = "RETURN"
	IptablesMark              string = "MARK"
	IptablesSrcFlag           string = "src"
	IptablesDstFlag           string = "dst"
	IptablesNotFlag           string = "!"
	IptablesProtFlag          string = "-p"
	IptablesSFlag             string = "-s"
	IptablesDFlag             string = "-d"
	IptablesDstPortFlag       string = "--dport"
	IptablesModuleFlag        string = "-m"
	IptablesSetModuleFlag     string = "set"
	IptablesMatchSetFlag      string = "--match-set"
	IptablesSetMarkFlag       string = "--set-mark"
	IptablesMarkFlag          string = "--mark"
	IptablesMarkVerb          string = "mark"
	IptablesStateModuleFlag   string = "state"
	IptablesStateFlag         string = "--state"
	IptablesMultiportFlag     string = "multiport"
	IptablesMultiDestportFlag string = "--dports"
	IptablesRelatedState      string = "RELATED"
	IptablesEstablishedState  string = "ESTABLISHED"
	IptablesFilterTable       string = "filter"
	IptablesCommentModuleFlag string = "comment"
	IptablesCommentFlag       string = "--comment"
	IptablesAddCommentFlag
	IptablesAzureChain             string = "AZURE-NPM"
	IptablesAzureAcceptChain       string = "AZURE-NPM-ACCEPT"
	IptablesAzureKubeSystemChain   string = "AZURE-NPM-KUBE-SYSTEM"
	IptablesAzureIngressChain      string = "AZURE-NPM-INGRESS"
	IptablesAzureIngressPortChain  string = "AZURE-NPM-INGRESS-PORT"
	IptablesAzureIngressFromChain  string = "AZURE-NPM-INGRESS-FROM"
	IptablesAzureEgressChain       string = "AZURE-NPM-EGRESS"
	IptablesAzureEgressPortChain   string = "AZURE-NPM-EGRESS-PORT"
	IptablesAzureEgressToChain     string = "AZURE-NPM-EGRESS-TO"
	IptablesKubeServicesChain      string = "KUBE-SERVICES"
	IptablesForwardChain           string = "FORWARD"
	IptablesInputChain             string = "INPUT"
	IptablesAzureIngressDropsChain string = "AZURE-NPM-INGRESS-DROPS"
	IptablesAzureEgressDropsChain  string = "AZURE-NPM-EGRESS-DROPS"
	// Below chain exists only in NPM before v1.2.6
	// TODO delete this below set while cleaning up
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
	IptablesAzureIngressMarkHex string = "0x2000"
	// IptablesAzureEgressXMarkHex is used for us to not override but append to the existing MARK
	// https://unix.stackexchange.com/a/283455 comment contains the explanation on
	// MARK manipulations with offset.
	IptablesAzureEgressXMarkHex string = "0x1000/0x1000"
	// IptablesAzureEgressMarkHex is for checking the absolute value of the mark
	IptablesAzureEgressMarkHex string = "0x1000"
	IptablesAzureAcceptMarkHex string = "0x3000"
	IptablesAzureClearMarkHex  string = "0x0"
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

	IpsetMaxelemName string = "maxelem"
	IpsetMaxelemNum  string = "4294967295"

	IpsetNomatch string = "nomatch"

	//Prefixes for ipsets
	NamedPortIPSetPrefix string = "namedport:"

	NamespacePrefix string = "ns-"
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

// These ID represents where did the error log generate from.
// It's for better query purpose. In Kusto these value are used in
// OperationID column
const (
	NpmID int = iota + 1
	IpsmID
	IptmID
	NSID
	PodID
	NetpolID
	UtilID
)
