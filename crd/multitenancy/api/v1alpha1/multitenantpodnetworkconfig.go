//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// MultitenantPodNetworkConfig is the Schema for the multitenantpodnetworkconfigs API
// +kubebuilder:resource:shortName=mtpnc,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="PodNetworkInstance",type=string,JSONPath=`.spec.podNetworkInstance`
// +kubebuilder:printcolumn:name="PodNetwork",type=string,JSONPath=`.spec.podNetwork`
// +kubebuilder:printcolumn:name="PodName",type=string,JSONPath=`.spec.podName`
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
}

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
	// DeviceType is the device type that this NC was created for
	DeviceType DeviceType `json:"deviceType,omitempty"`
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
}

func init() {
	SchemeBuilder.Register(&MultitenantPodNetworkConfig{}, &MultitenantPodNetworkConfigList{})
}
