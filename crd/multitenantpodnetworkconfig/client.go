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
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
		return nil, errors.Wrap(err, "failed to create mtpnc crd")
	}
	return res, nil
}

// Installs the embedded MultitenantPodNetworkConfig CRD definition in the cluster.
func (i *Installer) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	mtpnc, err := GetMultitenantPodNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded mtpnc crd")
	}
	return i.create(ctx, mtpnc)
}

// InstallOrUpdate installs the embedded MultitenantPodNetworkConfig CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdate(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	mtpnc, err := GetMultitenantPodNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded mtpnc crd")
	}
	current, err := i.create(ctx, mtpnc)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = i.cli.Get(ctx, mtpnc.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing mtpnc crd")
		}
	}
	if !reflect.DeepEqual(mtpnc.Spec.Versions, current.Spec.Versions) {
		mtpnc.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = i.cli.Update(ctx, mtpnc, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing mtpnc crd")
		}
	}
	return current, nil
}
