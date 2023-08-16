package podnetworkinstance

import (
	_ "embed"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// PodNetworkInstanceYAML embeds the CRD YAML for downstream consumers.
//
//go:embed manifests/acn.azure.com_podnetworkinstances.yaml
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
