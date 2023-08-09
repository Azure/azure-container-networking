package multitenantpodnetworkconfig

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/multitenantpodnetworkconfig/api/v1alpha1"
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

// Scheme is a runtime scheme containing the client-go scheme and the MultitenantPodNetworkConfig scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
}

// Installer provides methods to manage the lifecycle of the MultitenantPodNetworkConfig resource definition.
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
		return nil, errors.Wrap(err, "failed to create mpnc crd")
	}
	return res, nil
}

// Installs the embedded MultitenantPodNetworkConfig CRD definition in the cluster.
func (i *Installer) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	mpnc, err := GetMultitenantPodNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded mpnc crd")
	}
	return i.create(ctx, mpnc)
}

// InstallOrUpdate installs the embedded MultitenantPodNetworkConfig CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdate(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	mpnc, err := GetMultitenantPodNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded mpnc crd")
	}
	current, err := i.create(ctx, mpnc)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = i.cli.Get(ctx, mpnc.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing mpnc crd")
		}
	}
	if !reflect.DeepEqual(mpnc.Spec.Versions, current.Spec.Versions) {
		mpnc.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = i.cli.Update(ctx, mpnc, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing mpnc crd")
		}
	}
	return current, nil
}

// Client provides methods to interact with instances of the MultitenantPodNetworkConfig custom resource.
type Client struct {
	cli client.Client
}

// NewClient creates a new MultitenantPodNetworkConfig client around the passed ctrlcli.Client.
func NewClient(cli client.Client) *Client {
	return &Client{
		cli: cli,
	}
}

// Get returns the MultitenantPodNetworkConfig identified by the NamespacedName.
func (c *Client) Get(ctx context.Context, key types.NamespacedName) (*v1alpha1.MultitenantPodNetworkConfig, error) {
	multitenantPodNetworkConfig := &v1alpha1.MultitenantPodNetworkConfig{}
	err := c.cli.Get(ctx, key, multitenantPodNetworkConfig)
	return multitenantPodNetworkConfig, errors.Wrapf(err, "failed to get mpnc %v", key)
}

// PatchSpec performs a server-side patch of the passed MultitenantPodNetworkConfigSpec to the MultitenantPodNetworkConfig specified by the NamespacedName.
func (c *Client) PatchSpec(ctx context.Context, key types.NamespacedName, spec *v1alpha1.MultitenantPodNetworkConfigSpec, fieldManager string) (*v1alpha1.MultitenantPodNetworkConfig, error) {
	obj := genPatchSkel(key)
	obj.Spec = *spec
	if err := c.cli.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(fieldManager)); err != nil {
		return nil, errors.Wrap(err, "failed to patch mpnc")
	}
	return obj, nil
}

// UpdateSpec does a fetch, deepcopy, and update of the MultitenantPodNetworkConfig with the passed spec.
// Deprecated: UpdateSpec is deprecated and usage should migrate to PatchSpec.
func (c *Client) UpdateSpec(ctx context.Context, key types.NamespacedName, spec *v1alpha1.MultitenantPodNetworkConfigSpec) (*v1alpha1.MultitenantPodNetworkConfig, error) {
	mpnc, err := c.Get(ctx, key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mpnc")
	}
	spec.DeepCopyInto(&mpnc.Spec)
	if err := c.cli.Update(ctx, mpnc); err != nil {
		return nil, errors.Wrap(err, "failed to update mpnc")
	}
	return mpnc, nil
}

// SetOwnerRef sets the controller of the MultitenantPodNetworkConfig to the given object atomically, using HTTP Patch.
// Deprecated: SetOwnerRef is deprecated, use the more correctly named SetControllerRef.
func (c *Client) SetOwnerRef(ctx context.Context, key types.NamespacedName, owner metav1.Object, fieldManager string) (*v1alpha1.MultitenantPodNetworkConfig, error) {
	return c.SetControllerRef(ctx, key, owner, fieldManager)
}

// SetControllerRef sets the controller of the MultitenantPodNetworkConfig to the given object atomically, using HTTP Patch.
func (c *Client) SetControllerRef(ctx context.Context, key types.NamespacedName, owner metav1.Object, fieldManager string) (*v1alpha1.MultitenantPodNetworkConfig, error) {
	obj := genPatchSkel(key)
	if err := ctrlutil.SetControllerReference(owner, obj, Scheme); err != nil {
		return nil, errors.Wrapf(err, "failed to set controller reference for mpnc")
	}
	if err := c.cli.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(fieldManager)); err != nil {
		return nil, errors.Wrapf(err, "failed to patch mpnc")
	}
	return obj, nil
}

func genPatchSkel(key types.NamespacedName) *v1alpha1.MultitenantPodNetworkConfig {
	return &v1alpha1.MultitenantPodNetworkConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "MultitenantPodNetworkConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}
}
