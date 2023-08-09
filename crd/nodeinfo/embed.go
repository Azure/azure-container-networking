package nodeinfo

import (
	_ "embed"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// NodesInfoYAML embeds the CRD YAML for downstream consumers.
//
//go:embed manifests/acn.azure.com_nodesinfo.yaml
var NodesInfoYAML []byte

// GetNodesInfoDefinition parses the raw []byte NodesInfo in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetNodesInfo() (*apiextensionsv1.CustomResourceDefinition, error) {
	nodesInfo := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(NodesInfoYAML, &nodesInfo); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling embedded nodeInfo")
	}
	return nodesInfo, nil
}
