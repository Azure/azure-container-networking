package manifests

import (
	_ "embed"

	v1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"sigs.k8s.io/yaml"
)

// NodeNetworkConfigsYAML embeds the CRD YAML for downstream consumers.
//go:embed manifests/acn.azure.com_nodenetworkconfigs.yaml
var NodeNetworkConfigsYAML []byte

// GetNodeNetworkConfigsDefinition parses the raw []byte NodeNetworkConfigs in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetNodeNetworkConfigs() (*v1beta1.CustomResourceDefinition, error) {
	nodeNetworkConfigs := &v1beta1.CustomResourceDefinition{}
	return nodeNetworkConfigs, yaml.Unmarshal(NodeNetworkConfigsYAML, &nodeNetworkConfigs)
}
