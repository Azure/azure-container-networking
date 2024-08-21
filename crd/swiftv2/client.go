package swiftv2

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/swiftv2/api/v1beta1"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	typedv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// Scheme is a runtime scheme containing the client-go scheme and the MTPNC/NI scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(Scheme)
	_ = v1beta1.AddToScheme(Scheme)
}

// Installer provides methods to manage the lifecycle of the custom resource definition.
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
		return nil, errors.Wrap(err, "failed to create crd")
	}
	return res, nil
}

// Install installs the embedded PodNetwork CRD definition in the cluster.
func (i *Installer) InstallPodNetwork(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	podnetwork, err := GetPodNetworks()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded podnetwork crd")
	}
	return i.create(ctx, podnetwork)
}

// InstallOrUpdate installs the embedded PodNetwork CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdatePodNetwork(ctx context.Context) (*v1.CustomResourceDefinition, error) {
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

// Install installs the embedded WorkloadNetworkConfig CRD definition in the cluster.
func (i *Installer) InstallWorkloadNetworkConfig(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	workloadnetworkconfig, err := GetWorkloadNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded workloadnetworkconfig crd")
	}
	return i.create(ctx, workloadnetworkconfig)
}

// InstallOrUpdate installs the embedded WorkloadNetworkConfig CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdateWorkloadNetworkConfig(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	workloadnetworkconfig, err := GetWorkloadNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded workloadnetworkconfig crd")
	}
	current, err := i.create(ctx, workloadnetworkconfig)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = i.cli.Get(ctx, workloadnetworkconfig.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing workloadnetworkconfig crd")
		}
	}
	if !reflect.DeepEqual(workloadnetworkconfig.Spec.Versions, current.Spec.Versions) {
		workloadnetworkconfig.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = i.cli.Update(ctx, workloadnetworkconfig, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing workloadnetworkconfig crd")
		}
	}
	return current, nil
}
