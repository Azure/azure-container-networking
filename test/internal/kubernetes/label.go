package kubernetes

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type mergePayload struct {
	Metadata metadataMerge `json:"metadata"`
}

type metadataMerge struct {
	Labels map[string]string `json:"labels"`
}

// AddNodeLabels adds or replaces labels on a node.
func AddNodeLabels(ctx context.Context, nodes corev1.NodeInterface, nodeName string, labels map[string]string) (*apiv1.Node, error) {
	mergeData := mergePayload{Metadata: metadataMerge{Labels: labels}}

	bs, err := json.Marshal(mergeData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal labels")
	}

	node, err := nodes.Patch(ctx, nodeName, types.MergePatchType, bs, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to path nodes")
	}

	return node, nil
}
