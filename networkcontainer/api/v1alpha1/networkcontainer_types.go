/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// NetworkContainerSpec defines the desired state of NetworkContainer
type NetworkContainerSpec struct {
	// UUID - network container UUID
	UUID string `json:"uuid,omitempty"`
	// Network - customer VNet GUID
	Network string `json:"network,omitempty"`
	// Subnet - customer subnet name
	Subnet string `json:"subnet,omitempty"`
	// Node - kubernetes node name
	Node string `json:"node,omitempty"`
	// InterfaceName - the interface name for consuming Pod
	InterfaceName string `json:"interfaceName,omitempty"`
	// ReservationID - reservation ID for allocating IP
	ReservationID string `json:"reservationID,omitempty"`
}

// NetworkContainerStatus defines the observed state of NetworkContainer
type NetworkContainerStatus struct {
	// The IP address
	IP string `json:"ip,omitempty"`
	// The gateway IP address
	Gateway string `json:"gateway,omitempty"`
	// The state of network container
	State string `json:"state,omitempty"`
	// The subnet CIDR
	IPSubnet string `json:"ipSubnet,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkContainer is the Schema for the networkcontainers API
type NetworkContainer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkContainerSpec   `json:"spec,omitempty"`
	Status NetworkContainerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkContainerList contains a list of NetworkContainer
type NetworkContainerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkContainer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkContainer{}, &NetworkContainerList{})
}
