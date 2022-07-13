//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// ClusterSubnetState is the Schema for the ClusterSubnetState API
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
type ClusterSubnetState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSubnetStateSpec   `json:"spec,omitempty"`
	Status ClusterSubnetStateStatus `json:"status,omitempty"`
}

// ClusterSubnetStateSpec defines the desired state of ClusterSubnetState
type ClusterSubnetStateSpec struct {
	Timestamp string `json:"Timestamp"`
}

// ClusterSubnetStateStatus defines the observed state of ClusterSubnetState
type ClusterSubnetStateStatus struct {
	Status bool `json:"Status"`
}

// +kubebuilder:object:root=true

// ClusterSubnetStateList contains a list of ClusterSubnetState
type ClusterSubnetStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterSubnetState `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterSubnetState{}, &ClusterSubnetStateList{})
}
