package clustersubnetstate

import (
	_ "embed"

	// import the manifests package so that caller of this package have the manifests compiled in as a side-effect.
	_ "github.com/Azure/azure-container-networking/crd/clustersubnetstate/manifests"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// ClusterSubnetStatesYAML embeds the CRD YAML for downstream consumers.
//go:embed manifests/acn.azure.com_clustersubnetstates.yaml
var ClusterSubnetStatesYAML []byte

// GetClusterSubnetStatussDefinition parses the raw []byte ClusterSubnetStatuss in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetClusterSubnetStates() (*apiextensionsv1.CustomResourceDefinition, error) {
	clusterSubnetStates := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(ClusterSubnetStatesYAML, &clusterSubnetStates); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling embedded nnc")
	}
	return clusterSubnetStates, nil
}
