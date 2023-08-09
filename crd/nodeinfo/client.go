package nodeinfo

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/nodeinfo/api/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	typedv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Scheme is a runtime scheme containing the client-go scheme and the NodeInfo scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
}

// Installer provides methods to manage the lifecycle of the NodeInfo resource definition.
type Installer struct {
	cli typedv1.CustomResourceDefinitionInterface
}

func NewInstaller(c *rest.Config) (*Installer, error) {
	cli, err := crd.NewCRDClientFromConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init crd client")
	}
	return &Installer{
		cli: cli,
	}, nil
}

func (i *Installer) create(ctx context.Context, res *v1.CustomResourceDefinition) (*v1.CustomResourceDefinition, error) {
	res, err := i.cli.Create(ctx, res, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create nodeinfo crd")
	}
	return res, nil
}

// Install installs the embedded NodeInfo CRD definition in the cluster.
func (i *Installer) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nodeinfo, err := GetNodesInfo()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded nodeinfo crd")
	}
	return i.create(ctx, nodeinfo)
}

// InstallOrUpdate installs the embedded NodeInfo CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdate(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nodeinfo, err := GetNodesInfo()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded nodeinfo crd")
	}
	current, err := i.create(ctx, nodeinfo)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = i.cli.Get(ctx, nodeinfo.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing nodeinfo crd")
		}
	}
	if !reflect.DeepEqual(nodeinfo.Spec.Versions, current.Spec.Versions) {
		nodeinfo.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = i.cli.Update(ctx, nodeinfo, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing nodeinfo crd")
		}
	}
	return current, nil
}

// Client provides methods to interact with instances of the NodeInfo custom resource.
type Client struct {
	cli client.Client
}

// NewClient creates a new NodeInfo client around the passed ctrlcli.Client.
func NewClient(cli client.Client) *Client {
	return &Client{
		cli: cli,
	}
}

// Get returns the NodeInfo identified by the NamespacedName.
func (c *Client) Get(ctx context.Context, key types.NamespacedName) (*v1alpha1.NodeInfo, error) {
	nodeInfo := &v1alpha1.NodeInfo{}
	err := c.cli.Get(ctx, key, nodeInfo)
	return nodeInfo, errors.Wrapf(err, "failed to get nodeinfo %v", key)
}

// PatchSpec performs a server-side patch of the passed NodeInfoSpec to the NodeInfo specified by the NamespacedName.
func (c *Client) PatchSpec(ctx context.Context, key types.NamespacedName, spec *v1alpha1.NodeInfoSpec, fieldManager string) (*v1alpha1.NodeInfo, error) {
	obj := genPatchSkel(key)
	obj.Spec = *spec
	if err := c.cli.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(fieldManager)); err != nil {
		return nil, errors.Wrap(err, "failed to patch nodeinfo")
	}
	return obj, nil
}

// UpdateSpec does a fetch, deepcopy, and update of the NodeInfo with the passed spec.
// Deprecated: UpdateSpec is deprecated and usage should migrate to PatchSpec.
func (c *Client) UpdateSpec(ctx context.Context, key types.NamespacedName, spec *v1alpha1.NodeInfoSpec) (*v1alpha1.NodeInfo, error) {
	nodeinfo, err := c.Get(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nodeinfo")
	}
	spec.DeepCopyInto(&nodeinfo.Spec)
	if err := c.cli.Update(ctx, nodeinfo); err != nil {
		return nil, errors.Wrap(err, "failed to update nodeinfo")
	}
	return nodeinfo, nil
}

// SetOwnerRef sets the controller of the NodeInfo to the given object atomically, using HTTP Patch.
// Deprecated: SetOwnerRef is deprecated, use the more correctly named SetControllerRef.
func (c *Client) SetOwnerRef(ctx context.Context, key types.NamespacedName, owner metav1.Object, fieldManager string) (*v1alpha1.NodeInfo, error) {
	return c.SetControllerRef(ctx, key, owner, fieldManager)
}

// SetControllerRef sets the controller of the NodeInfo to the given object atomically, using HTTP Patch.
func (c *Client) SetControllerRef(ctx context.Context, key types.NamespacedName, owner metav1.Object, fieldManager string) (*v1alpha1.NodeInfo, error) {
	obj := genPatchSkel(key)
	if err := ctrlutil.SetControllerReference(owner, obj, Scheme); err != nil {
		return nil, errors.Wrapf(err, "failed to set controller reference for nodeinfo")
	}
	if err := c.cli.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(fieldManager)); err != nil {
		return nil, errors.Wrapf(err, "failed to patch nodeinfo")
	}
	return obj, nil
}

func genPatchSkel(key types.NamespacedName) *v1alpha1.NodeInfo {
	return &v1alpha1.NodeInfo{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "NodeInfo",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}
}
