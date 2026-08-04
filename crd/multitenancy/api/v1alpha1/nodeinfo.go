//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// NodeInfo is the Schema for the NodeInfo API
// +kubebuilder:resource:shortName=ni,scope=Cluster,path=nodeinfo
// +kubebuilder:printcolumn:name="VMUniqueID",type=string,priority=0,JSONPath=`.spec.vmUniqueID`
type NodeInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NodeInfoSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// NodeInfoList contains a list of NodeInfo
type NodeInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeInfo `json:"items"`
}

// NodeInfoSpec defines the desired state of NodeInfo
type NodeInfoSpec struct {
	// +kubebuilder:validation:Optional
	VMUniqueID string `json:"vmUniqueID,omitempty"`
}

func init() {
	SchemeBuilder.Register(&NodeInfo{}, &NodeInfoList{})
}
