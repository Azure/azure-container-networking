package nodenetworkconfig

import (
	"context"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlcli "sigs.k8s.io/controller-runtime/pkg/client"
)

var Scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = v1alpha.AddToScheme(Scheme)
}

type Client interface {
	PatchSpec(context.Context, *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error)
}

type client struct {
	types.NamespacedName
	cli ctrlcli.Client
}

// NewClient will return a NodeNetworkConfig Client that can interact with the
// NNC identified by the passed NamespacedName key, backed a default controller client.
func NewClient(config *rest.Config, key types.NamespacedName) (Client, error) {
	opts := ctrlcli.Options{
		Scheme: Scheme,
	}
	cli, err := ctrlcli.New(config, opts)
	if err != nil {
		return nil, err
	}
	return &client{
		NamespacedName: key,
		cli:            cli,
	}, nil
}

// NewWithClient will return a NodeNetworkConfig Client that can interact with the
// NNC identified by the passed NamespacedName key, backed by the passed controller client.
func NewWithClient(cli ctrlcli.Client, key types.NamespacedName) Client {
	return &client{
		NamespacedName: key,
		cli:            cli,
	}
}

// PatchSpec performs a server-side patch of the object identified by the passed key, updating just the passed Spec.
func (c *client) PatchSpec(ctx context.Context, spec *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error) {
	obj := &v1alpha.NodeNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
	}

	patch, err := specToJSON(spec)
	if err != nil {
		return nil, err
	}

	if err := c.cli.Patch(ctx, obj, ctrlcli.RawPatch(types.ApplyPatchType, patch)); err != nil {
		return nil, err
	}

	return obj, nil
}

func specToJSON(spec *v1alpha.NodeNetworkConfigSpec) ([]byte, error) {
	m := map[string]*v1alpha.NodeNetworkConfigSpec{
		"spec": spec,
	}
	return json.Marshal(m)
}
