//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// PodNetworkInstance is the Schema for the PodNetworkInstances API
// +kubebuilder:resource:shortName=pni,scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="Pod IPs",type=string,priority=1,JSONPath=`.status.podIPAddresses`
// +kubebuilder:printcolumn:name="PodNetwork",type=string,priority=1,JSONPath=`.spec.podNetwork`
// +kubebuilder:printcolumn:name="PodIPReservationSize",type=string,priority=1,JSONPath=`.spec.podIPReservationSize`
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

// PodNetworkInstanceSpec defines the desired state of PodNetworkInstance
type PodNetworkInstanceSpec struct {
	// pod network resource object name
	PodNetwork string `json:"podnetwork"`
	// number of backend IP address to reserve for running pods
	// +kubebuilder:default=0
	PodIPReservationSize int `json:"podIPReservationSize"`
}

// PodNetworkInstanceStatus defines the observed state of PodNetworkInstance
type PodNetworkInstanceStatus struct {
	PodIPAddresses []string `json:"podIPAddresses,omitempty"`
}

func init() {
	SchemeBuilder.Register(&PodNetworkInstance{}, &PodNetworkInstanceList{})
}
