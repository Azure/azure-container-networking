package nodeinfo

import (
	_ "embed"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// NodeInfoYAML embeds the CRD YAML for downstream consumers.
//
//go:embed manifests/acn.azure.com_nodeinfo.yaml
var NodeInfoYAML []byte

// GetNodeInfo parses the raw []byte NodeInfo in
// to a CustomResourceDefinition and returns it or an unmarshalling error.
func GetNodeInfo() (*apiextensionsv1.CustomResourceDefinition, error) {
	nodeInfo := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(NodeInfoYAML, &nodeInfo); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling embedded nodeInfo")
	}
	return nodeInfo, nil
}
