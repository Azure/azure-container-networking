package multitenantpodnetworkconfig

import (
	_ "embed"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// MultitenantPodNetworkConfigsYAML embeds the CRD YAML for downstream consumers.
//
//go:embed manifests/acn.azure.com_multitenantpodnetworkconfigs.yaml
var MultitenantPodNetworkConfigsYAML []byte

// GetMultitenantPodNetworkConfigs parses the raw []byte MultitenantPodNetworkConfigs in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetMultitenantPodNetworkConfigs() (*apiextensionsv1.CustomResourceDefinition, error) {
	multitenantPodNetworkConfigs := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(MultitenantPodNetworkConfigsYAML, &multitenantPodNetworkConfigs); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling embedded mtpnc")
	}
	return multitenantPodNetworkConfigs, nil
}
