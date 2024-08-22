//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// PodNetwork is the Schema for the PodNetworks API
// +kubebuilder:resource:shortName=pn,scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,priority=0,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Address Prefixes",type=string,priority=1,JSONPath=`.status.addressPrefixes`
// +kubebuilder:printcolumn:name="Network",type=string,priority=0,JSONPath=`.spec.networkID`
// +kubebuilder:printcolumn:name="Subnet",type=string,priority=1,JSONPath=`.spec.subnetResourceID`
// +kubebuilder:printcolumn:name="SubnetGUID",type=string,priority=0,JSONPath=`.spec.subnetGUID`
// +kubebuilder:printcolumn:name="DeviceType",type=string,priority=0,JSONPath=`.spec.deviceType`
type PodNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Spec is immutable"
	Spec   PodNetworkSpec   `json:"spec,omitempty"`
	Status PodNetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodNetworkList contains a list of PodNetwork
type PodNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodNetwork `json:"items"`
}

// +kubebuilder:validation:Enum=acn.azure.com/vnet-nic;acn.azure.com/infiniband-nic
type DeviceType string

const (
	DeviceTypeVnetNIC       DeviceType = "acn.azure.com/vnet-nic"
	DeviceTypeInfiniBandNIC DeviceType = "acn.azure.com/infiniband-nic"
)

// PodNetworkSpec defines the desired state of PodNetwork
type PodNetworkSpec struct {
	// NetworkID is the identifier for the network, e.g. vnet guid or IB network ID
	NetworkID string `json:"networkID,omitempty"`
	// DeviceType is the device type that is required by this network
	DeviceType DeviceType `json:"deviceType,omitempty"`
	// customer subnet id
	// +kubebuilder:validation:Optional
	SubnetResourceID string `json:"subnetResourceID,omitempty"`
	// customer subnet guid
	// +kubebuilder:validation:Optional
	SubnetGUID string `json:"subnetGUID,omitempty"`
}

// Status indicates the status of PN
// +kubebuilder:validation:Enum=Ready;InUse;SubnetNotDelegated;SubnetDelegatedToDifferentService
type Status string

const (
	Ready                             Status = "Ready"
	InUse                             Status = "InUse"
	SubnetNotDelegated                Status = "SubnetNotDelegated"
	SubnetDelegatedToDifferentService Status = "SubnetDelegatedToDifferentService"
)

// PodNetworkStatus defines the observed state of PodNetwork
type PodNetworkStatus struct {
	// +kubebuilder:validation:Optional
	Status          Status   `json:"status,omitempty"`
	AddressPrefixes []string `json:"addressPrefixes,omitempty"`
}

func init() {
	SchemeBuilder.Register(&PodNetwork{}, &PodNetworkList{})
}
