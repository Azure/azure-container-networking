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
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Scheme is a runtime scheme containing the client-go scheme and the ClusterSubnetStatus scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
}

// Installer provides methods to manage the lifecycle of the ClusterSubnetState resource definition.
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
		return nil, errors.Wrap(err, "failed to create css crd")
	}
	return res, nil
}

// Install installs the embedded ClusterSubnetState CRD definition in the cluster.
func (i *Installer) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	css, err := GetClusterSubnetStates()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded css crd")
	}
	return i.create(ctx, css)
}

// InstallOrUpdate installs the embedded ClusterSubnetState CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdate(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	css, err := GetClusterSubnetStates()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded css crd")
	}
	current, err := i.create(ctx, css)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = i.cli.Get(ctx, css.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing css crd")
		}
	}
	if !reflect.DeepEqual(css.Spec.Versions, current.Spec.Versions) {
		css.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = i.cli.Update(ctx, css, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing css crd")
		}
	}
	return current, nil
}

// Client provides methods to interact with instances of the ClusterSubnetState custom resource.
type Client struct {
	cli client.Client
}

// NewClient creates a new ClusterSubnetState client from the passed ctrlcli.Client.
func NewClient(cli client.Client) *Client {
	return &Client{
		cli: cli,
	}
}

// Get returns the ClusterSubnetState identified by the NamespacedName.
func (c *Client) Get(ctx context.Context, key types.NamespacedName) (*v1alpha1.ClusterSubnetState, error) {
	clusterSubnetState := &v1alpha1.ClusterSubnetState{}
	err := c.cli.Get(ctx, key, clusterSubnetState)
	return clusterSubnetState, errors.Wrapf(err, "failed to get css %v", key)
}
