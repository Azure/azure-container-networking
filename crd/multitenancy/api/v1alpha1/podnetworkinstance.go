//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// PodNetworkInstance is the Schema for the PodNetworkInstances API
// +kubebuilder:resource:shortName=pni,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="PodNetworks",priority=1,type=string,JSONPath=`.spec.podNetworkConfigs[*].podNetwork`
type PodNetworkInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodNetworkInstanceSpec   `json:"spec,omitempty"`
	Status PodNetworkInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodNetworkInstanceList contains a list of PodNetworkInstance
type PodNetworkInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodNetworkInstance `json:"items"`
}

// PodNetworkConfig describes a template for how to attach a PodNetwork to a Pod
type PodNetworkConfig struct {
	// PodNetwork is the name of a PodNetwork resource
	PodNetwork string `json:"podNetwork"`
	// PodIPReservationSize is the number of IP address to statically reserve
	// +kubebuilder:default=0
	PodIPReservationSize int `json:"podIPReservationSize,omitempty"`
}

// PodNetworkInstanceSpec defines the desired state of PodNetworkInstance
type PodNetworkInstanceSpec struct {
	// Deprecated - use PodNetworks
	// +kubebuilder:validation:Optional
	PodNetwork string `json:"podnetwork,omitempty"`
	// Deprecated - use PodNetworks
	// +kubebuilder:default=0
	PodIPReservationSize int `json:"podIPReservationSize,omitempty"`
	// PodNetworkConfigs describes each PodNetwork to attach to a single Pod
	// optional for now in case orchestrator uses the deprecated fields
	// +kubebuilder:validation:Optional
	PodNetworkConfigs []PodNetworkConfig `json:"podNetworkConfigs"`
	// DefaultDenyACL bool indicates whether default deny policy will be present on the pods upon pod creation
	// +kubebuilder:default=false
	// +kubebuilder:validation:Optional
	DefaultDenyACL bool `json:"defaultDenyACL"`
}

// PodNetworkInstanceStatus defines the observed state of PodNetworkInstance
type PodNetworkInstanceStatus struct {
	// +kubebuilder:validation:Optional
	PodIPAddresses     []string             `json:"podIPAddresses,omitempty"`
	Status             PNIStatus            `json:"status,omitempty"`
	PodNetworkStatuses map[string]PNIStatus `json:"podNetworkStatuses,omitempty"`
}

// PNIStatus indicates the status of PNI
// +kubebuilder:validation:Enum=Ready;CreateReservationSetError;PodNetworkNotReady;InsufficientIPAddressesOnSubnet
type PNIStatus string

const (
	PNIStatusReady                           PNIStatus = "Ready"
	PNIStatusCreateReservationSetError       PNIStatus = "CreateReservationSetError"
	PNIStatusPodNetworkNotReady              PNIStatus = "PodNetworkNotReady"
	PNIStatusInsufficientIPAddressesOnSubnet PNIStatus = "InsufficientIPAddressesOnSubnet"
)

func init() {
	SchemeBuilder.Register(&PodNetworkInstance{}, &PodNetworkInstanceList{})
}
