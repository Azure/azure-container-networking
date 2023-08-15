package podnetwork

import (
	_ "embed"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// PodNetworkYAML embeds the CRD YAML for downstream consumers.
//
//go:embed manifests/acn.azure.com_podnetworks.yaml
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
