package clustersubnetstate

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
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
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Scheme is a runtime scheme containing the client-go scheme and the ClusterSubnetStatus scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
}

// Client is provided to interface with the ClusterSubnetState CRDs.
type Client struct {
	csscli ctrlcli.Client
	crdcli typedv1.CustomResourceDefinitionInterface
}

// NewClient creates a new ClusterSubnetState client from the passed k8s Config.
func NewClient(c *rest.Config) (*Client, error) {
	crdCli, err := crd.NewCRDClient(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init crd client")
	}
	opts := ctrlcli.Options{
		Scheme: Scheme,
	}
	cssCli, err := ctrlcli.New(c, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init css client")
	}
	return &Client{
		crdcli: crdCli,
		csscli: cssCli,
	}, nil
}

func (c *Client) create(ctx context.Context, res *v1.CustomResourceDefinition) (*v1.CustomResourceDefinition, error) {
	res, err := c.crdcli.Create(ctx, res, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create css crd")
	}
	return res, nil
}

// Get returns the ClusterSubnetState identified by the NamespacedName.
func (c *Client) Get(ctx context.Context, key types.NamespacedName) (*v1alpha1.ClusterSubnetState, error) {
	clusterSubnetState := &v1alpha1.ClusterSubnetState{}
	err := c.csscli.Get(ctx, key, clusterSubnetState)
	return clusterSubnetState, errors.Wrapf(err, "failed to get css %v", key)
}

// Install installs the embedded ClusterSubnetState CRD definition in the cluster.
func (c *Client) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	css, err := GetClusterSubnetStates()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded css crd")
	}
	return c.create(ctx, css)
}

// InstallOrUpdate installs the embedded ClusterSubnetState CRD definition in the cluster or updates it if present.
func (c *Client) InstallOrUpdate(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	css, err := GetClusterSubnetStates()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded css crd")
	}
	current, err := c.create(ctx, css)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = c.crdcli.Get(ctx, css.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing css crd")
		}
	}
	if !reflect.DeepEqual(css.Spec.Versions, current.Spec.Versions) {
		css.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = c.crdcli.Update(ctx, css, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing css crd")
		}
	}
	return current, nil
}

// SetOwnerRef sets the owner of the ClusterSubnetStatus to the given object, using HTTP Patch
func (c *Client) SetOwnerRef(ctx context.Context, key types.NamespacedName, owner metav1.Object, fieldManager string) (*v1alpha1.ClusterSubnetState, error) {
	obj := genPatchSkel(key)
	if err := ctrlutil.SetControllerReference(owner, obj, Scheme); err != nil {
		return nil, errors.Wrapf(err, "failed to set controller reference for css")
	}
	if err := c.csscli.Patch(ctx, obj, ctrlcli.Apply, ctrlcli.ForceOwnership, ctrlcli.FieldOwner(fieldManager)); err != nil {
		return nil, errors.Wrapf(err, "failed to patch css")
	}
	return obj, nil
}

func genPatchSkel(key types.NamespacedName) *v1alpha1.ClusterSubnetState {
	return &v1alpha1.ClusterSubnetState{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "ClusterSubnetState",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
	}
}
