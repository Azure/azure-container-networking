//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1beta1

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
// +kubebuilder:printcolumn:name="PodName",type=string,JSONPath=`.spec.podName`
// +kubebuilder:unservedversion
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
	PodNetworkInstance string `json:"podNetworkInstance,omitempty"`
	// name of the requesting cx pod
	PodName string `json:"podName,omitempty"`
}

type InterfaceInfo struct {
	// name of PN object from requesting interface
	PodNetwork string `json:"podNetwork,omitempty"`
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
	// Routes is a list of routes to add to the Pod through interface
	// +kubebuilder:default={}
	Routes []string `json:"routes,omitempty"`
	// PolicyBasedRouting is a flag to enable policy based routing
	// +kubebuilder:default=true
	PolicyBasedRouting bool `json:"policyBasedRouting,omitempty"`
}

// MultitenantPodNetworkConfigStatus defines the observed state of PodNetworkConfig
type MultitenantPodNetworkConfigStatus struct {
	// InterfaceInfos describes all of the network container goal state for this Pod
	// +kubebuilder:validation:Optional
	InterfaceInfos []InterfaceInfo `json:"interfaceInfos,omitempty"`
	// ClusterNetworkConfig describes how to attach the infra network to a Pod
	// +kubebuilder:validation:Optional
	ClusterNetworkConfig []ClusterNetworkConfig `json:"clusterNetworkConfig,omitempty"`
}

func init() {
	SchemeBuilder.Register(&MultitenantPodNetworkConfig{}, &MultitenantPodNetworkConfigList{})
}
