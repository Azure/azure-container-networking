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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// AvailableIPMapping groups an IP address and Name. This is a struct and not a Map for future extensibility
type AvailableIPMapping struct {
	Name string `json:"name,omitempty"`
	IP   string `json:"ip,omitempty"`
}

// NetworkContainerSpec defines the structure of a Network Container as found in NetworkConfigStatus
type NetworkContainerSpec struct {
	PrimaryIP      string               `json:"primaryIP,omitempty"`
	ID             string               `json:"id,omitempty"`
	Subnet         string               `json:"subnet,omitempty"`
	AvailableIPs   []AvailableIPMapping `json:"availableIPs,omitempty"`
	DefaultGateway string               `json:"defaultGateway,omitempty"`
	Netmask        string               `json:"netmask,omitempty"`
}

// NetworkConfigSpec defines the desired state of NetworkConfig
type NetworkConfigSpec struct {
	Count       int64    `json:"count,omitempty"`
	IPsNotInUse []string `json:"ipsNotInUse,omitempty"`
}

// NetworkConfigStatus defines the observed state of NetworkConfig
type NetworkConfigStatus struct {
	Count      int64                  `json:"count,omitempty"`
	BufferSize int64                  `json:"bufferSize,omitempty"`
	NCs        []NetworkContainerSpec `json:"ncs,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkConfig is the Schema for the networkconfigs API
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
type NetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkConfigSpec   `json:"spec,omitempty"`
	Status NetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkConfigList contains a list of NetworkConfig
type NetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkConfig{}, &NetworkConfigList{})
}
