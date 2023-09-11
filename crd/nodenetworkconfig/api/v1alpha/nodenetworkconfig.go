//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1alpha

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// +kubebuilder:object:root=true

// NodeNetworkConfig is the Schema for the nodenetworkconfigs API
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:resource:shortName=nnc
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Requested IPs",type=integer,priority=1,JSONPath=`.spec.requestedIPCount`
// +kubebuilder:printcolumn:name="Allocated IPs",type=integer,priority=0,JSONPath=`.status.assignedIPCount`
// +kubebuilder:printcolumn:name="Subnet",type=string,priority=1,JSONPath=`.status.networkContainers[*].subnetName`
// +kubebuilder:printcolumn:name="Subnet CIDR",type=string,priority=1,JSONPath=`.status.networkContainers[*].subnetAddressSpace`
// +kubebuilder:printcolumn:name="NC ID",type=string,priority=1,JSONPath=`.status.networkContainers[*].id`
// +kubebuilder:printcolumn:name="NC Mode",type=string,priority=0,JSONPath=`.status.networkContainers[*].assignmentMode`
// +kubebuilder:printcolumn:name="NC Type",type=string,priority=1,JSONPath=`.status.networkContainers[*].type`
// +kubebuilder:printcolumn:name="NC Version",type=integer,priority=0,JSONPath=`.status.networkContainers[*].version`
type NodeNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeNetworkConfigSpec   `json:"spec,omitempty"`
	Status NodeNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeNetworkConfigList contains a list of NetworkConfig
type NodeNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeNetworkConfig `json:"items"`
}

// NodeNetworkConfigSpec defines the desired state of NetworkConfig
type NodeNetworkConfigSpec struct {
	// +kubebuilder:default=0
	// +kubebuilder:validation:Optional
	RequestedIPCount int64    `json:"requestedIPCount"`
	IPsNotInUse      []string `json:"ipsNotInUse,omitempty"`
}

// Status indicates the NNC reconcile status
// +kubebuilder:validation:Enum=Updating;Updated;Error
type Status string

const (
	Updating Status = "Updating"
	Updated  Status = "Updated"
	Error    Status = "Error"
)

// NCStatus indicates the latest NC request status
// +kubebuilder:validation:Enum=SubnetFull
// +kubebuilder:validation:Optional
type NCStatus string

const (
	NCStatusSubnetFull NCStatus = "SubnetFull"
)

// NodeNetworkConfigStatus defines the observed state of NetworkConfig
type NodeNetworkConfigStatus struct {
	// +kubebuilder:default=0
	// +kubebuilder:validation:Optional
	AssignedIPCount   int                `json:"assignedIPCount"`
	Scaler            Scaler             `json:"scaler,omitempty"`
	Status            Status             `json:"status,omitempty"`
	NetworkContainers []NetworkContainer `json:"networkContainers,omitempty"`
}

// Scaler groups IP request params together
type Scaler struct {
	BatchSize               int64 `json:"batchSize,omitempty"`
	ReleaseThresholdPercent int64 `json:"releaseThresholdPercent,omitempty"`
	RequestThresholdPercent int64 `json:"requestThresholdPercent,omitempty"`
	MaxIPCount              int64 `json:"maxIPCount,omitempty"`
}

// AssignmentMode is whether we are allocated an entire block or IP by IP.
// +kubebuilder:validation:Enum=dynamic;static
type AssignmentMode string

const (
	Dynamic AssignmentMode = "dynamic"
	Static  AssignmentMode = "static"
)

// NCType is the specific type of network this NC represents.
type NCType string

const (
	VNET      NCType = "vnet"
	VNETBlock NCType = "vnetblock"
	Overlay   NCType = "overlay"
)

// NetworkContainer defines the structure of a Network Container as found in NetworkConfigStatus
type NetworkContainer struct {
	ID string `json:"id,omitempty"`
	// +kubebuilder:default=dynamic
	AssignmentMode AssignmentMode `json:"assignmentMode,omitempty"`
	// +kubebuilder:default=vnet
	Type               NCType         `json:"type,omitempty"`
	PrimaryIP          string         `json:"primaryIP,omitempty"`
	SubnetName         string         `json:"subnetName,omitempty"`
	IPAssignments      []IPAssignment `json:"ipAssignments,omitempty"`
	DefaultGateway     string         `json:"defaultGateway,omitempty"`
	SubnetAddressSpace string         `json:"subnetAddressSpace,omitempty"`
	// +kubebuilder:default=0
	// +kubebuilder:validation:Optional
	Version         int64    `json:"version"`
	NodeIP          string   `json:"nodeIP,omitempty"`
	SubscriptionID  string   `json:"subcriptionID,omitempty"`
	ResourceGroupID string   `json:"resourceGroupID,omitempty"`
	VNETID          string   `json:"vnetID,omitempty"`
	SubnetID        string   `json:"subnetID,omitempty"`
	Status          NCStatus `json:"status,omitempty"`
}

// IPAssignment groups an IP address and Name. Name is a UUID set by the the IP address assigner.
type IPAssignment struct {
	Name string `json:"name,omitempty"`
	IP   string `json:"ip,omitempty"`
}

func init() {
	SchemeBuilder.Register(&NodeNetworkConfig{}, &NodeNetworkConfigList{})
}
