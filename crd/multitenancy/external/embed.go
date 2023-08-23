package external

import (
	_ "embed"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// PodNetworkYAML embeds the CRD YAML for downstream consumers.
//
//go:embed manifests/public.acn.azure.com_podnetworks.yaml
var PodNetworkYAML []byte

// GetPodNetworks parses the raw []byte PodNetwork in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetPodNetworks() (*apiextensionsv1.CustomResourceDefinition, error) {
	podNetworks := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(PodNetworkYAML, &podNetworks); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling embedded PodNetwork")
	}
	return podNetworks, nil
}

// PodNetworkInstanceYAML embeds the CRD YAML for downstream consumers.
//
//go:embed manifests/public.acn.azure.com_podnetworkinstances.yaml
var PodNetworkInstanceYAML []byte

// GetPodNetworkInstances parses the raw []byte PodNetworkInstance in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetPodNetworkInstances() (*apiextensionsv1.CustomResourceDefinition, error) {
	podNetworkInstances := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(PodNetworkInstanceYAML, &podNetworkInstances); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling embedded podNetworkInstance")
	}
	return podNetworkInstances, nil
}
