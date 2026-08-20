//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// UVMNetworkConfig is the Schema for the UVMNetworkConfig API.
//
// It represents tenant-level InfiniBand (IB) programming intent for a node/UVM,
// decoupled from pod lifecycle. One UVMNetworkConfig exists per node/UVM and
// covers all IB NICs on that node - see the design invariant:
//
//	one node -> one UVM -> one backend network -> all IB NICs on that node
//
// The CRD is created by the tenant-aware scheduler when the first tenant
// workload is placed onto a UVM, and deleted when the tenant/UVM lifecycle
// ends. DNC-RC reconciles it using the existing NC create/delete plumbing;
// pod create/delete events must never affect it.
// +kubebuilder:resource:shortName=uvmnc,scope=Cluster,path=uvmnetworkconfigs
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BackendNetwork",type=string,JSONPath=`.spec.backendNetwork.name`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
type UVMNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UVMNetworkConfigSpec   `json:"spec,omitempty"`
	Status UVMNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UVMNetworkConfigList contains a list of UVMNetworkConfig.
type UVMNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UVMNetworkConfig `json:"items"`
}

// BackendNetworkReference identifies the single backend network a node's IB
// NICs are associated with. No MAC addresses are supplied here - the platform
// (DNC-RC) enumerates the node's IB NICs itself (via NodeInfo) and associates
// all of them with this backend network.
type BackendNetworkReference struct {
	// Name of the backend network.
	Name string `json:"name"`
}

// UVMNetworkConfigSpec defines the desired state of UVMNetworkConfig.
//
// Note: partitionKey is deliberately not expressed here. PKey allocation is
// owned by NSM and is an implementation detail of the fabric programming
// layer, not a tenant-supplied input. It may be surfaced in status for
// observability.
type UVMNetworkConfigSpec struct {
	// BackendNetwork is the single backend network associated with all IB
	// NICs on this node.
	BackendNetwork BackendNetworkReference `json:"backendNetwork"`
}

// +kubebuilder:validation:Enum=Pending;Programming;Programmed;Deleting;Failed
type UVMNetworkConfigState string

const (
	UVMNetworkConfigStatePending     UVMNetworkConfigState = "Pending"
	UVMNetworkConfigStateProgramming UVMNetworkConfigState = "Programming"
	UVMNetworkConfigStateProgrammed  UVMNetworkConfigState = "Programmed"
	UVMNetworkConfigStateDeleting    UVMNetworkConfigState = "Deleting"
	UVMNetworkConfigStateFailed      UVMNetworkConfigState = "Failed"
)

// UFMPartitionStatus reports the UFM partition programming status for the
// backend network associated with this node.
type UFMPartitionStatus struct {
	// BackendNetwork is the backend network this UFM partition status applies to.
	// +kubebuilder:validation:Optional
	BackendNetwork string `json:"backendNetwork,omitempty"`
	// State is the UFM partition state (e.g. Active).
	// +kubebuilder:validation:Optional
	State string `json:"state,omitempty"`
}

// UVMNetworkConfigStatus defines the observed state of UVMNetworkConfig.
type UVMNetworkConfigStatus struct {
	// State is the overall IB programming state for this node/UVM.
	// +kubebuilder:validation:Optional
	State UVMNetworkConfigState `json:"state,omitempty"`
	// UFMPartition reports the UFM partition programming status.
	// +kubebuilder:validation:Optional
	UFMPartition UFMPartitionStatus `json:"ufmPartition,omitempty"`
	// Conditions represent the latest available observations of the
	// UVMNetworkConfig's state.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// LastUpdated is the last time the status was updated.
	// +kubebuilder:validation:Optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// UVMNetworkConfigCondition types.
const (
	// UVMNetworkConfigConditionUFMProgrammed indicates whether the UFM
	// partition for the node's backend network has been programmed.
	UVMNetworkConfigConditionUFMProgrammed = "UFMProgrammed"
)

func init() {
	SchemeBuilder.Register(&UVMNetworkConfig{}, &UVMNetworkConfigList{})
}
