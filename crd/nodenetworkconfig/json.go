package nodenetworkconfig

import (
	"encoding/json"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

func specToJSON(spec *v1alpha.NodeNetworkConfigSpec) ([]byte, error) {
	m := map[string]*v1alpha.NodeNetworkConfigSpec{
		"spec": spec,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal nnc spec")
	}
	return b, nil
}
