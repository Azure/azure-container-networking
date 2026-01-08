//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// PodNetwork is the Schema for the PodNetworks API
// +kubebuilder:resource:shortName=pn,scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,priority=1,JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Address Prefixes",type=string,priority=1,JSONPath=`.status.addressPrefixes`
// +kubebuilder:printcolumn:name="Network",type=string,priority=1,JSONPath=`.spec.networkID`
// +kubebuilder:printcolumn:name="Subnet",type=string,priority=1,JSONPath=`.spec.subnetResourceID`
// +kubebuilder:printcolumn:name="SubnetGUID",type=string,priority=1,JSONPath=`.spec.subnetGUID`
// +kubebuilder:printcolumn:name="DeviceType",type=string,priority=1,JSONPath=`.spec.deviceType`
type PodNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

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

// +kubebuilder:validation:Enum=Single;PrefixBlock
type SubnetAllocationMode string

const (
	Single      SubnetAllocationMode = "Single"
	PrefixBlock SubnetAllocationMode = "PrefixBlock"
)

// PodNetworkSpec defines the desired state of PodNetwork
type PodNetworkSpec struct {
	// NetworkID is the identifier for the network, e.g. vnet guid or IB network ID
	// +kubebuilder:validation:Optional
	NetworkID string `json:"networkID,omitempty"`
	// DeviceType is the device type that is required by this network
	// +kubebuilder:validation:Optional
	DeviceType DeviceType `json:"deviceType,omitempty"`
	// customer subnet id
	// +kubebuilder:validation:Optional
	SubnetResourceID string `json:"subnetResourceID,omitempty"`
	// customer subnet guid
	// +kubebuilder:validation:Optional
	SubnetGUID string `json:"subnetGUID,omitempty"`
	// Deprecated - Use NetworkID
	// +kubebuilder:validation:Optional
	VnetGUID string `json:"vnetGUID,omitempty"`
	// SubnetAllocationMode is the mode chosen by user for subnet allocation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Single
	SubnetAllocationMode SubnetAllocationMode `json:"subnetAllocationMode,omitempty"`
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
	// PrefixBlockAllocationSize indicates the size of IP block allocated to the subnet
	// +kubebuilder:default=16
	PrefixBlockAllocationSize int `json:"prefixBlockAllocationSize,omitempty"`
}

func init() {
	SchemeBuilder.Register(&PodNetwork{}, &PodNetworkList{})
}
