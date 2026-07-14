//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// NICNetworkConfig is the Schema for the nicnetworkconfigs API
// +kubebuilder:resource:shortName=nicnc,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="MACAddress",type=string,JSONPath=`.spec.macAddress`
// +kubebuilder:printcolumn:name="PodNetwork",type=string,JSONPath=`.spec.podNetwork`
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
	// PodNetwork is the name of the PodNetwork
	PodNetwork string `json:"podNetwork"`
	// NodeName is the name of the node this NIC belongs to
	NodeName string `json:"nodeName"`
	// MACAddress is the MAC address of the NIC, used to create the network container
	MACAddress string `json:"macAddress"`
	// customer subnet id
	SubnetResourceID string `json:"subnetResourceID"`
	// customer subnet guid
	// +kubebuilder:validation:Optional
	SubnetGUID string `json:"subnetGUID,omitempty"`
	// NetworkID is the VNET GUID or network identifier
	NetworkID string `json:"networkID"`
	// ScheduledByDRA indicates the pod was scheduled via Dynamic Resource Allocation (DRA).
	// +kubebuilder:validation:Optional
	ScheduledByDRA bool `json:"scheduledByDRA,omitempty"`
	// PodAllocationRequests tracks which pods are allocated on this NIC
	// +kubebuilder:validation:Optional
	PodAllocationRequests []PodAllocationRequest `json:"podAllocations,omitempty"`
}

// PodAllocationRequest represents a pod's IP allocation request on this NIC
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
	// +kubebuilder:validation:Enum=Ready;Pending;Deleting;InternalError;NCCreateError
	Status NICNCStatus `json:"status,omitempty"`
	// NCID is the network container id created for this NIC
	// +kubebuilder:validation:Optional
	NCID string `json:"ncID,omitempty"`
	// PrimaryIP is the primary IP allocated to the network container
	// +kubebuilder:validation:Optional
	PrimaryIP string `json:"primaryIP,omitempty"`
	// MACAddress is the MAC Address of the VM's NIC
	MACAddress string `json:"macAddress,omitempty"`
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
	// CooldownPeriodInSeconds is the cooldown duration before retrying NIC NC operations.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=0
	CooldownPeriodInSeconds int `json:"cooldownPeriodInSeconds,omitempty"`
	// DeviceType is the device type that this NC was created for
	DeviceType DeviceType `json:"deviceType,omitempty"`
	// AccelnetEnabled determines if the CNI will provision the NIC with accelerated networking enabled
	// +kubebuilder:validation:Optional
	AccelnetEnabled bool `json:"accelnetEnabled,omitempty"`
}

// NICNCStatus indicates the status of NIC Network Config
type NICNCStatus string

const (
	// NICNCReady indicates the NIC's network container has been successfully created and is ready for use.
	NICNCReady NICNCStatus = "Ready"
	// NICNCPending indicates the NIC's network container is awaiting processing.
	NICNCPending NICNCStatus = "Pending"
	// NICNCDeleting indicates the NIC's network container is being cleaned up.
	NICNCDeleting NICNCStatus = "Deleting"
	// NICNCInternalError indicates an internal error occurred while processing the NIC Network Config.
	NICNCInternalError NICNCStatus = "InternalError"
	// NICNCNCCreateError indicates the network container creation for the NIC failed.
	NICNCNCCreateError NICNCStatus = "NCCreateError"
)

func init() {
	SchemeBuilder.Register(&NICNetworkConfig{}, &NICNetworkConfigList{})
}
