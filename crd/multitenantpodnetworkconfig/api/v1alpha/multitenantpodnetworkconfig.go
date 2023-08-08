//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// MultitenantPodNetworkConfig is the Schema for the multitenantpodnetworkconfigs API
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=mpnc
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="PodNetworkInstance",type=string,JSONPath=`.spec.podNetworkInstance`
// +kubebuilder:printcolumn:name="PodName",type=string,JSONPath=`.spec.podName`
// +kubebuilder:printcolumn:name="UUID",type=string,JSONPath=`.status.uuid`
// +kubebuilder:printcolumn:name="PrimaryIP",type=string,JSONPath=`.status.primaryIP`
// +kubebuilder:printcolumn:name="MacAddress",type=string,JSONPath=`.status.macAddress`
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
	PodNetworkInstance string `json:"podNetworkInstance,omitempty"`
	PodName            string `json:"podName,omitempty"`
}

// MultitenantPodNetworkConfigStatus defines the observed state of PodNetworkConfig
type MultitenantPodNetworkConfigStatus struct {
	UUID       string `json:"uuid,omitempty"`
	PrimaryIP  string `json:"primaryIP,omitempty"`
	MacAddress string `json:"macAddress,omitempty"`
}

func init() {
	SchemeBuilder.Register(&MultitenantPodNetworkConfig{}, &MultitenantPodNetworkConfigList{})
}
