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
// +kubebuilder:printcolumn:name="NCID",type=string,JSONPath=`.status.ncID`
// +kubebuilder:printcolumn:name="PrimaryIP",type=string,JSONPath=`.status.primaryIP`
// +kubebuilder:printcolumn:name="MacAddress",type=string,JSONPath=`.status.macAddress`
// +kubebuilder:printcolumn:name="GatewayIP",type=string,JSONPath=`.status.gatewayIP`
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
	PodName string `json:"podName"`
}

// MultitenantPodNetworkConfigStatus defines the observed state of PodNetworkConfig
type MultitenantPodNetworkConfigStatus struct {
	// network container id
	NCID string `json:"ncID,omitempty"`
	// ip allocated to the network container
	PrimaryIP string `json:"primaryIP,omitempty"`
	// maps to the NIC to be injected for the network container
	MacAddress string `json:"macAddress,omitempty"`
	// Gateway IP
	GatewayIP string `json:"gatewayIP,omitempty"`
}

func init() {
	SchemeBuilder.Register(&MultitenantPodNetworkConfig{}, &MultitenantPodNetworkConfigList{})
}
