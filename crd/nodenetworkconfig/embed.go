package nodenetworkconfig

import (
	_ "embed"

	// import the manifests package so that caller of this package have the manifests compiled in as a side-effect.
	_ "github.com/Azure/azure-container-networking/crd/nodenetworkconfig/manifests"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// NodeNetworkConfigsYAML embeds the CRD YAML for downstream consumers.
//go:embed manifests/acn.azure.com_nodenetworkconfigs.yaml
var NodeNetworkConfigsYAML []byte

// GetNodeNetworkConfigsDefinition parses the raw []byte NodeNetworkConfigs in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetNodeNetworkConfigs() (*apiextensionsv1.CustomResourceDefinition, error) {
	nodeNetworkConfigs := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(NodeNetworkConfigsYAML, &nodeNetworkConfigs); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling embedded nnc")
	}
	return nodeNetworkConfigs, nil
}
