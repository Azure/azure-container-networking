//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// NICNetworkConfig is the Schema for the nicnetworkconfigs API
// +kubebuilder:resource:shortName=nnc,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="NICID",type=string,JSONPath=`.spec.nicID`
// +kubebuilder:printcolumn:name="NCID",type=string,JSONPath=`.spec.ncID`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
type NICNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NICNetworkConfigSpec   `json:"spec,omitempty"`
	Status NICNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NICNetworkConfigList contains a list of NICNetworkConfig
type NICNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NICNetworkConfig `json:"items"`
}

// NICNetworkConfigSpec defines the desired state of NICNetworkConfig
type NICNetworkConfigSpec struct {
	// NodeName is the name of the Kubernetes node
	NodeName string `json:"nodeName"`
	// PodNetwork is the name of the pod network
	PodNetwork string `json:"podNetwork"`
	// VNetID is the Azure Virtual Network ID
	VnetID string `json:"vnetID"`
	// PodAllocations tracks which pods are allocated on this NIC
	// +kubebuilder:validation:Optional
	PodAllocations []PodAllocationRequest `json:"podAllocations,omitempty"`
}

// PodAllocation represents a pod's IP allocation on this NIC
type PodAllocationRequest struct {
	// PodName is the name of the pod
	PodName string `json:"podName"`
	// PodNamespace is the namespace of the pod
	PodNamespace string `json:"podNamespace"`
	// MTPNC is the name of the MultitenantPodNetworkConfig
	MTPNC string `json:"mtpnc"`
}

// PodAllocation represents a pod's IP allocation on this NIC
type PodAllocation struct {
	// PodName is the name of the pod
	PodName string `json:"podName"`
	// PodNamespace is the namespace of the pod
	PodNamespace string `json:"podNamespace"`
	// AllocatedIP is the IP address allocated to the pod
	AllocatedIP string `json:"allocatedIP"`
	// MTPNC is the name of the MultitenantPodNetworkConfig
	MTPNC string `json:"mtpnc"`
}

// NICNetworkConfigStatus defines the observed state of NICNetworkConfig
type NICNetworkConfigStatus struct {
	// Status indicates the current status of the NIC Network Config
	// +kubebuilder:validation:Enum=Ready;Pending;Error
	Status NNCStatus `json:"status,omitempty"`
	// NCID is the network container id created for this NIC
	// +kubebuilder:validation:Optional
	NCID string `json:"ncID,omitempty"`
	// PrimaryIP is the primary IP allocated to the network container
	// +kubebuilder:validation:Optional
	PrimaryIP string `json:"primaryIP,omitempty"`
	// MacAddress is the MAC Address of the VM's NIC
	MacAddress string `json:"macAddress,omitempty"`
	// GatewayIP is the gateway ip of the injected subnet
	// +kubebuilder:validation:Optional
	GatewayIP string `json:"gatewayIP,omitempty"`
	// SubnetAddressSpace is the subnet address space of the injected subnet
	// +kubebuilder:validation:Optional
	SubnetAddressSpace string `json:"subnetAddressSpace,omitempty"`
	// AvailableIPs tracks the available IP addresses in this NC block
	// +kubebuilder:validation:Optional
	AvailableIPs []string `json:"availableIPs,omitempty"`
	// PodAllocations tracks the allocated IP addresses to pod mapping.
	// +kubebuilder:validation:Optional
	PodAllocations map[string]PodAllocation `json:"podAllocations,omitempty"`
	// ErrorMessage contains error details if status is Error
	// +kubebuilder:validation:Optional
	ErrorMessage string `json:"errorMessage,omitempty"`
	// DeviceType is the device type that this NC was created for
	DeviceType DeviceType `json:"deviceType,omitempty"`
	// AccelnetEnabled determines if the CNI will provision the NIC with accelerated networking enabled
	// +kubebuilder:validation:Optional
	AccelnetEnabled bool `json:"accelnetEnabled,omitempty"`
}

// NNCStatus indicates the status of NIC Network Config
type NNCStatus string

const (
	NNCStatusReady   NNCStatus = "Ready"
	NNCStatusPending NNCStatus = "Pending"
	NNCStatusError   NNCStatus = "Error"
)

func init() {
	SchemeBuilder.Register(&NICNetworkConfig{}, &NICNetworkConfigList{})
}
