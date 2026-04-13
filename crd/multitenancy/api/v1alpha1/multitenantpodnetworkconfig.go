//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// MultitenantPodNetworkConfig is the Schema for the multitenantpodnetworkconfigs API
// +kubebuilder:resource:shortName=mtpnc,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="PodNetworkInstance",type=string,JSONPath=`.spec.podNetworkInstance`
// +kubebuilder:printcolumn:name="PodName",type=string,JSONPath=`.spec.podName`
// +kubebuilder:printcolumn:name="PodUID",type=string,JSONPath=`.spec.podUID`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
type MultitenantPodNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultitenantPodNetworkConfigSpec   `json:"spec,omitempty"`
	Status MultitenantPodNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MultitenantPodNetworkConfigList contains a list of PodNetworkConfig
type MultitenantPodNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultitenantPodNetworkConfig `json:"items"`
}

// MultitenantPodNetworkConfigSpec defines the desired state of PodNetworkConfig
type MultitenantPodNetworkConfigSpec struct {
	// name of PNI object from requesting cx pod
	// +kubebuilder:validation:Optional
	PodNetworkInstance string `json:"podNetworkInstance,omitempty"`
	// name of PN object from requesting cx pod
	PodNetwork string `json:"podNetwork"`
	// name of the requesting cx pod
	PodName string `json:"podName,omitempty"`
	// MAC addresses of the IB devices to use for a pod
	// +kubebuilder:validation:Optional
	IBMACAddresses []string `json:"IBMACAddresses,omitempty"`
	// PodUID is the UID of the pod
	PodUID types.UID `json:"podUID,omitempty"`
}

// +kubebuilder:validation:Enum=Unprogrammed;Programming;Programmed;Unprogramming;Failed
type InfinibandStatus string

const (
	Unprogrammed  InfinibandStatus = "Unprogrammed"
	Programming   InfinibandStatus = "Programming"
	Programmed    InfinibandStatus = "Programmed"
	Unprogramming InfinibandStatus = "Unprogramming"
	Failed        InfinibandStatus = "Failed"
)

// MTPNCStatus indicates the high-level status of MultitenantPodNetworkConfig
// +kubebuilder:validation:Enum=Ready;Pending;InternalError;PNINotFound;PNINotReady;NodeCapacityExceeded;IPsExhausted;Deleting
type MTPNCStatus string

const (
	// MTPNCStatusReady indicates the MTPNC has been successfully programmed and is ready for use
	MTPNCStatusReady MTPNCStatus = "Ready"
	// MTPNCStatusPending indicates the MTPNC is awaiting processing
	MTPNCStatusPending MTPNCStatus = "Pending"
	// MTPNCStatusInternalError indicates an internal error occurred while processing the MTPNC
	MTPNCStatusInternalError MTPNCStatus = "InternalError"
	// MTPNCStatusPNINotFound indicates the referenced PodNetworkInstance was not found
	MTPNCStatusPNINotFound MTPNCStatus = "PNINotFound"
	// MTPNCStatusPNINotReady indicates the referenced PodNetworkInstance is not yet ready
	MTPNCStatusPNINotReady MTPNCStatus = "PNINotReady"
	// MTPNCStatusNodeCapacityExceeded indicates the node has exceeded its capacity for network resources
	MTPNCStatusNodeCapacityExceeded MTPNCStatus = "NodeCapacityExceeded"
	// MTPNCStatusIPsExhausted indicates no IP addresses are available for allocation
	MTPNCStatusIPsExhausted MTPNCStatus = "IPsExhausted"
	// MTPNCStatusDeleting indicates MTPNC is being deleted, status may not be set at the same time as deletionTimestamp.
	MTPNCStatusDeleting MTPNCStatus = "Deleting"
)

type InterfaceInfo struct {
	// NCID is the network container id
	NCID string `json:"ncID,omitempty"`
	// PrimaryIP is the ip allocated to the network container
	// +kubebuilder:validation:Optional
	PrimaryIP string `json:"primaryIP,omitempty"`
	// MacAddress is the MAC Address of the VM's NIC which this network container was created for
	MacAddress string `json:"macAddress,omitempty"`
	// GatewayIP is the gateway ip of the injected subnet
	// +kubebuilder:validation:Optional
	GatewayIP string `json:"gatewayIP,omitempty"`
	// SubnetAddressSpace is the subnet address space of the injected subnet
	// +kubebuilder:validation:Optional
	SubnetAddressSpace string `json:"subnetAddressSpace,omitempty"`
	// DeviceType is the device type that this NC was created for
	DeviceType DeviceType `json:"deviceType,omitempty"`
	// AccelnetEnabled determines if the CNI will provision the NIC with accelerated networking enabled
	// +kubebuilder:validation:Optional
	AccelnetEnabled bool `json:"accelnetEnabled,omitempty"`
	// IBStatus is the programming status of the infiniband device
	// +kubebuilder:validation:Optional
	IBStatus InfinibandStatus `json:"ibStatus,omitempty"`
	// SharedNIC is a field for prefixblock mode,
	// it determines if CNI will need to keep this NIC in the pod / node namespace
	// +kubebuilder:validation:Optional
	SharedNIC bool `json:"sharedNic,omitempty"`
}

// MultitenantPodNetworkConfigStatus defines the observed state of PodNetworkConfig
type MultitenantPodNetworkConfigStatus struct {
	// Deprecated - use InterfaceInfos
	// +kubebuilder:validation:Optional
	NCID string `json:"ncID,omitempty"`
	// Deprecated - use InterfaceInfos
	// +kubebuilder:validation:Optional
	PrimaryIP string `json:"primaryIP,omitempty"`
	// Deprecated - use InterfaceInfos
	// +kubebuilder:validation:Optional
	MacAddress string `json:"macAddress,omitempty"`
	// Deprecated - use InterfaceInfos
	// +kubebuilder:validation:Optional
	GatewayIP string `json:"gatewayIP,omitempty"`
	// InterfaceInfos describes all of the network container goal state for this Pod
	// +kubebuilder:validation:Optional
	InterfaceInfos []InterfaceInfo `json:"interfaceInfos,omitempty"`
	// DefaultDenyACL bool indicates whether default deny policy will be present on the pods upon pod creation
	// +kubebuilder:validation:Optional
	DefaultDenyACL bool `json:"defaultDenyACL"`
	// NodeName is the name of the node where the pod is scheduled
	// +kubebuilder:validation:Optional
	NodeName string `json:"nodeName,omitempty"`
	// Status represents the overall status of the MTPNC
	// +kubebuilder:validation:Optional
	Status MTPNCStatus `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&MultitenantPodNetworkConfig{}, &MultitenantPodNetworkConfigList{})
}
