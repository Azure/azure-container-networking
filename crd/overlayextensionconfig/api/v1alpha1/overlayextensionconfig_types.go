//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OverlayExtensionConfig is the Schema for the overlayextensionconfigs API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=oec
// +kubebuilder:printcolumn:name="OverlayExtensionConfig IP range",type=string,priority=1,JSONPath=`.spec.extensionIPRange`
type OverlayExtensionConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OverlayExtensionConfigSpec   `json:"spec,omitempty"`
	Status OverlayExtensionConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OverlayExtensionConfigList contains a list of OverlayExtensionConfig
type OverlayExtensionConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OverlayExtensionConfig `json:"items"`
}

// OverlayExtensionConfigSpec defines the desired state of OverlayExtensionConfig.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.extensionIPRange) || has(self.extensionIPRange)", message="ExtensionIPRange is required once set"
type OverlayExtensionConfigSpec struct {
	// ExtensionIPRange field defines a CIDR that should be able to reach routing domain ip addresses.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Value is immutable"
	// +kubebuilder:validation:MaxLength=43
	// 43 is max length of IPv6 CIDR string
	ExtensionIPRange string `json:"extensionIPRange,omitempty"`
}

type OECState string

const (
	None      OECState = "None"
	Pending   OECState = "Pending"
	Succeeded OECState = "Succeeded"
	Failed    OECState = "Failed"
)

// OverlayExtensionConfigStatus defines the observed state of OverlayExtensionConfig
type OverlayExtensionConfigStatus struct {
	// +kubebuilder:validation:Enum=None;Pending;Succeeded;Failed
	// +kubebuilder:default="None"
	State   OECState `json:"state,omitempty"`
	Message string   `json:"message,omitempty"`
}

func init() {
	SchemeBuilder.Register(&OverlayExtensionConfig{}, &OverlayExtensionConfigList{})
}
