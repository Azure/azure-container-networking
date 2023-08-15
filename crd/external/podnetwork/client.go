package podnetwork

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/external/podnetwork/api/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	typedv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// Scheme is a runtime scheme containing the client-go scheme and the PodNetwork scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
}

// Installer provides methods to manage the lifecycle of the PodNetwork resource definition.
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
		return nil, errors.Wrap(err, "failed to create podnetwork crd")
	}
	return res, nil
}

// Install installs the embedded PodNetwork CRD definition in the cluster.
func (i *Installer) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	podnetwork, err := GetPodNetworks()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded podnetwork crd")
	}
	return i.create(ctx, podnetwork)
}

// InstallOrUpdate installs the embedded PodNetwork CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdate(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	podNetwork, err := GetPodNetworks()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded podnetwork crd")
	}
	current, err := i.create(ctx, podNetwork)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = i.cli.Get(ctx, podNetwork.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing podnetwork crd")
		}
	}
	if !reflect.DeepEqual(podNetwork.Spec.Versions, current.Spec.Versions) {
		podNetwork.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = i.cli.Update(ctx, podNetwork, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing podnetwork crd")
		}
	}
	return current, nil
}
