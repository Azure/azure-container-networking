package types

import (
	networkingv1 "k8s.io/api/networking/v1"
	corev1 "k8s.io/api/core/v1"
)

// Severity levels for audit findings
type Severity string

const (
	SeverityFail Severity = "FAIL"
	SeverityWarn Severity = "WARN"
	SeverityPass Severity = "PASS"
	SeverityInfo Severity = "INFO"
)

// RuleID identifies a specific breaking change detection rule
type RuleID string

const (
	RuleIPBlockCatchAll         RuleID = "CILIUM-001"
	RuleNamedPorts              RuleID = "CILIUM-002"
	RuleEndPort                 RuleID = "CILIUM-003"
	RuleImplicitLocalNodeEgress RuleID = "CILIUM-004"
	RuleLBIngressEnforcement    RuleID = "CILIUM-005"
	RuleHostNetworkPods         RuleID = "CILIUM-006"
	RuleKubeProxyRemoval        RuleID = "CILIUM-007"
	RuleIdentityExhaustion      RuleID = "CILIUM-008"
	RuleServiceMeshDetected     RuleID = "CILIUM-009"
)

// Finding represents a single audit finding for a NetworkPolicy
type Finding struct {
	RuleID      RuleID   `json:"ruleId"`
	Severity    Severity `json:"severity"`
	PolicyName  string   `json:"policyName"`
	Namespace   string   `json:"namespace"`
	Description string   `json:"description"`
	Remediation string   `json:"remediation"`
	AffectedFields []string `json:"affectedFields,omitempty"`
}

// AuditReport is the full output of the policy analyzer
type AuditReport struct {
	ClusterName    string    `json:"clusterName,omitempty"`
	Timestamp      string    `json:"timestamp"`
	TotalPolicies  int       `json:"totalPolicies"`
	Findings       []Finding `json:"findings"`
	Summary        AuditSummary `json:"summary"`
}

// AuditSummary provides aggregate counts
type AuditSummary struct {
	FailCount int `json:"failCount"`
	WarnCount int `json:"warnCount"`
	PassCount int `json:"passCount"`
	InfoCount int `json:"infoCount"`
}

// ClusterResources holds all the Kubernetes objects we need for analysis
type ClusterResources struct {
	NetworkPolicies []networkingv1.NetworkPolicy
	Pods            []corev1.Pod
	Services        []corev1.Service
	Nodes           []corev1.Node
	Namespaces      []corev1.Namespace
}

// TranslationOutput holds the generated patches and CiliumNetworkPolicies
type TranslationOutput struct {
	PatchedPolicies    []PatchedPolicy    `json:"patchedPolicies"`
	CiliumPolicies     []CiliumPolicy     `json:"ciliumPolicies"`
	RemovedNamedPorts  []NamedPortMapping `json:"removedNamedPorts,omitempty"`
}

// PatchedPolicy is a modified Kubernetes NetworkPolicy
type PatchedPolicy struct {
	Original *networkingv1.NetworkPolicy `json:"original"`
	Patched  *networkingv1.NetworkPolicy `json:"patched"`
	Changes  []string                    `json:"changes"`
}

// CiliumPolicy represents a generated CiliumNetworkPolicy
type CiliumPolicy struct {
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Reason    string                 `json:"reason"`
	Spec      map[string]interface{} `json:"spec"`
}

// NamedPortMapping records the resolution of a named port to its numeric value
type NamedPortMapping struct {
	PolicyName string `json:"policyName"`
	Namespace  string `json:"namespace"`
	PortName   string `json:"portName"`
	PortNumber int32  `json:"portNumber"`
	Protocol   string `json:"protocol"`
}

// ConnectivityProbe represents a single connectivity test
type ConnectivityProbe struct {
	SourceNamespace string `json:"sourceNamespace"`
	SourcePod       string `json:"sourcePod"`
	TargetAddress   string `json:"targetAddress"`
	TargetPort      int    `json:"targetPort"`
	Protocol        string `json:"protocol"`
	ProbeType       string `json:"probeType"` // pod-to-pod, pod-to-service, pod-to-external, pod-to-node
}

// ConnectivityResult is the outcome of a single probe
type ConnectivityResult struct {
	Probe     ConnectivityProbe `json:"probe"`
	Success   bool              `json:"success"`
	LatencyMs int64             `json:"latencyMs,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// ConnectivitySnapshot is the full pre/post migration connectivity state
type ConnectivitySnapshot struct {
	ClusterName string               `json:"clusterName"`
	Timestamp   string               `json:"timestamp"`
	Phase       string               `json:"phase"` // "pre-migration" or "post-migration"
	Results     []ConnectivityResult `json:"results"`
}

// ConnectivityDiff shows what changed between pre and post migration
type ConnectivityDiff struct {
	PreSnapshot  *ConnectivitySnapshot `json:"preSnapshot"`
	PostSnapshot *ConnectivitySnapshot `json:"postSnapshot"`
	Regressions  []ConnectivityResult  `json:"regressions"`  // was working, now broken
	NewAllows    []ConnectivityResult  `json:"newAllows"`     // was blocked, now working
	Unchanged    int                   `json:"unchanged"`
}

// ClusterInfo holds discovery information about an AKS cluster
type ClusterInfo struct {
	Name            string `json:"name"`
	ResourceGroup   string `json:"resourceGroup"`
	SubscriptionID  string `json:"subscriptionId"`
	Location        string `json:"location"`
	KubernetesVersion string `json:"kubernetesVersion"`
	NetworkPlugin   string `json:"networkPlugin"`
	NetworkPolicy   string `json:"networkPolicy"`
	NetworkDataplane string `json:"networkDataplane"`
	NodeCount       int    `json:"nodeCount"`
	HasWindowsPools bool   `json:"hasWindowsPools"`
	PolicyCount     int    `json:"policyCount,omitempty"`
	RiskLevel       string `json:"riskLevel,omitempty"` // low, medium, high
	MigrationOrder  int    `json:"migrationOrder,omitempty"`
}

// MigrationPlan is the fleet-level migration ordering
type MigrationPlan struct {
	Timestamp string        `json:"timestamp"`
	Clusters  []ClusterInfo `json:"clusters"`
	Summary   FleetSummary  `json:"summary"`
}

// FleetSummary provides fleet-level stats
type FleetSummary struct {
	TotalClusters     int `json:"totalClusters"`
	ReadyToMigrate    int `json:"readyToMigrate"`
	NeedsRemediation  int `json:"needsRemediation"`
	BlockedByWindows  int `json:"blockedByWindows"`
}

// MigrationState tracks the progress of a single cluster migration
type MigrationState struct {
	ClusterName     string `json:"clusterName"`
	ResourceGroup   string `json:"resourceGroup"`
	Phase           string `json:"phase"` // preflight, snapshot, migrating, patching, validating, complete, failed
	StartTime       string `json:"startTime"`
	EndTime         string `json:"endTime,omitempty"`
	PreSnapshot     string `json:"preSnapshotPath,omitempty"`
	PostSnapshot    string `json:"postSnapshotPath,omitempty"`
	PatchesApplied  int    `json:"patchesApplied"`
	Error           string `json:"error,omitempty"`
}
