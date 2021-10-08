package nodenetworkconfig

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	typedv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlcli "sigs.k8s.io/controller-runtime/pkg/client"
)

// Scheme is a runtime scheme containing the client-go scheme and the NodeNetworkConfig scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = v1alpha.AddToScheme(Scheme)
}

// Client is provided to interface with the NodeNetworkConfig CRDs.
type Client struct {
	nnccli ctrlcli.Client
	crdcli typedv1.CustomResourceDefinitionInterface
}

// NewClient creates a new NodeNetworkConfig client from the passed k8s Config.
func NewClient(c *rest.Config) (*Client, error) {
	crdCli, err := crd.NewCRDClient(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init crd client")
	}
	opts := ctrlcli.Options{
		Scheme: Scheme,
	}
	nnnCli, err := ctrlcli.New(c, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init nnc client")
	}
	return &Client{
		crdcli: crdCli,
		nnccli: nnnCli,
	}, nil
}

func (c *Client) create(ctx context.Context, res *v1.CustomResourceDefinition) (*v1.CustomResourceDefinition, error) {
	res, err := c.crdcli.Create(ctx, res, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create nnc crd")
	}
	return res, nil
}

// Get returns the NodeNetworkConfig identified by the NamespacedName.
func (c *Client) Get(ctx context.Context, key types.NamespacedName) (*v1alpha.NodeNetworkConfig, error) {
	nodeNetworkConfig := &v1alpha.NodeNetworkConfig{}
	err := c.nnccli.Get(ctx, key, nodeNetworkConfig)
	return nodeNetworkConfig, errors.Wrapf(err, "failed to get nnc %v", key)
}

// Install installs the embedded NodeNetworkConfig CRD definition in the cluster.
func (c *Client) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nnc, err := GetNodeNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded nnc crd")
	}
	return c.create(ctx, nnc)
}

// InstallOrUpdate installs the embedded NodeNetworkConfig CRD definition in the cluster or updates it if present.
func (c *Client) InstallOrUpdate(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nnc, err := GetNodeNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded nnc crd")
	}
	current, err := c.create(ctx, nnc)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = c.crdcli.Get(ctx, nnc.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing nnc crd")
		}
	}
	if !reflect.DeepEqual(nnc.Spec.Versions, current.Spec.Versions) {
		nnc.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = c.crdcli.Update(ctx, nnc, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing nnc crd")
		}
	}
	return current, nil
}

// PatchSpec performs a server-side patch of the passed NodeNetworkConfigSpec to the NodeNetworkConfig specified by the NamespacedName.
func (c *Client) PatchSpec(ctx context.Context, key types.NamespacedName, spec *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error) {
	obj := &v1alpha.NodeNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}

	patch, err := specToJSON(spec)
	if err != nil {
		return nil, err
	}

	if err := c.nnccli.Patch(ctx, obj, ctrlcli.RawPatch(types.ApplyPatchType, patch)); err != nil {
		return nil, errors.Wrap(err, "failed to patch nnc")
	}

	return obj, nil
}

// UpdateSpec does a fetch, deepcopy, and update of the NodeNetworkConfig with the passed spec.
// Deprecated: UpdateSpec is deprecated and usage should migrate to PatchSpec.
func (c *Client) UpdateSpec(ctx context.Context, key types.NamespacedName, spec *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error) {
	nnc, err := c.Get(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nnc")
	}
	spec.DeepCopyInto(&nnc.Spec)
	if err := c.nnccli.Update(ctx, nnc); err != nil {
		return nil, errors.Wrap(err, "failed to update nnc")
	}
	return nnc, nil
}
