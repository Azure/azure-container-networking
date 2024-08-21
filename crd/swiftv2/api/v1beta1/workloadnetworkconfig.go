//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// WorkloadNetworkConfig is the Schema for the WorkloadNetworkConfigs API
// +kubebuilder:resource:shortName=wnc,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels=managed=
// +kubebuilder:metadata:labels=owner=
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
type WorkloadNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadNetworkConfigSpec   `json:"spec,omitempty"`
	Status WorkloadNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadNetworkConfigList contains a list of WorkloadNetworkConfig
type WorkloadNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadNetworkConfig `json:"items"`
}

// PodNetworkConfig describes a template for how to attach a PodNetwork to a Pod
// +kubebuilder:validation:XValidation:rule="self.policyBasedRouting || self.routes.size() > 0",message="routes list shouldn't be empty if policybasedRouting is disabled."
type PodNetworkConfig struct {
	// PodNetwork is the name of a PodNetwork resource
	// +kubebuilder:validation:MaxLength=100
	PodNetwork string `json:"podNetwork"`
	// PodIPReservationSize is the number of IP address to statically reserve
	// +kubebuilder:default=0
	PodIPReservationSize int `json:"podIPReservationSize,omitempty"`
	// Routes is a list of routes to add to the Pod through interface assigned to this PodNetwork
	// +kubebuilder:default={}
	Routes []string `json:"routes,omitempty"`
	// PolicyBasedRouting is a flag to enable policy based routing
	// +kubebuilder:default=true
	PolicyBasedRouting bool `json:"policyBasedRouting,omitempty"`
}

// ClusterNetworkConfig describes a template for how to attach the infra network to a Pod
// +kubebuilder:validation:XValidation:rule="self.policyBasedRouting || self.routes.size() > 0",message="Routes list shouldn't be empty if policybasedRouting is disabled."
type ClusterNetworkConfig struct {
	// +kubebuilder:default={}
	// Routes is a list of routes to add to the Pod through interface assigned to the infra network
	Routes []string `json:"routes,omitempty"`
	// PolicyBasedRouting is a flag to enable policy based routing
	// +kubebuilder:default=true
	PolicyBasedRouting bool `json:"policyBasedRouting,omitempty"`
}

// WorkloadNetworkConfigSpec defines the desired state of WorkloadNetworkConfig
type WorkloadNetworkConfigSpec struct {
	// ClusterNetworkConfig describes how to attach the infra network to a Pod
	ClusterNetworkConfig ClusterNetworkConfig `json:"clusterNetworkConfig"`
	// PodNetworkConfigs describes each PodNetwork to attach to a single Pod
	// +kubebuilder:validation:MaxItems=10
	// +kubebuilder:validation:XValidation:rule="self.size() == oldSelf.size()",message="Count of PodNetworkConfigs is immutable"
	//nolint:lll // Explanation: kubebuilder markers don't fold into multiple lines
	// +kubebuilder:validation:XValidation:rule="self.all(podNetworkConfig, oldSelf.exists(oldPodNetworkConfig, oldPodNetworkConfig.podNetwork == podNetworkConfig.podNetwork && oldPodNetworkConfig.podIPReservationSize == podNetworkConfig.podIPReservationSize))",message="podNetwork and podIPReservationSize in podNetworkConfig are immutable"
	PodNetworkConfigs []PodNetworkConfig `json:"podNetworkConfigs"`
}

// PodNetworkConfigStatus is the status of the PodNetworkConfig
type PodNetworkConfigStatus struct {
	// +kubebuilder:validation:Optional
	Status PNCStatus `json:"status,omitempty"`
	// +kubebuilder:validation:Optional
	PodIPAddresses []string `json:"podIPAddresses,omitempty"`
}

// WorkloadNetworkConfigStatus defines the observed state of WorkloadNetworkConfig
type WorkloadNetworkConfigStatus struct {
	// Status indicates the status of WNC
	Status WNCStatus `json:"status,omitempty"`
	// PodNetworkConfigStatuses describes the status of each PodNetworkConfig
	// +kubebuilder:validation:Optional
	PodNetworkConfigStatuses []PodNetworkConfigStatus `json:"podNetworkConfigStatuses,omitempty"`
}

// PNCStatus indicates the status of individual PodNetworkConfig
// +kubebuilder:validation:Enum=Ready;CreateReservationSetError;PodNetworkNotReady;InsufficientIPAddressesOnSubnet
type PNCStatus string

const (
	PNCStatusReady                           PNCStatus = "Ready"
	PNCStatusCreateReservationSetError       PNCStatus = "CreateReservationSetError"
	PNCStatusPodNetworkNotReady              PNCStatus = "PodNetworkNotReady"
	PNCStatusInsufficientIPAddressesOnSubnet PNCStatus = "InsufficientIPAddressesOnSubnet"
)

// WNCStatus indicates the status of WorkloadNetworkConfig
// +kubebuilder:validation:Enum=Ready;NotReady
type WNCStatus string

const (
	WNCStatusReady    WNCStatus = "Ready"
	WNCStatusNotReady WNCStatus = "NotReady"
)

func init() {
	SchemeBuilder.Register(&WorkloadNetworkConfig{}, &WorkloadNetworkConfigList{})
}
