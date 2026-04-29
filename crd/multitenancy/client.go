package multitenancy

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	typedv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Scheme is a runtime scheme containing the client-go scheme and the MTPNC/NI scheme.
var Scheme = runtime.NewScheme()

func init() {
	_ = scheme.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
}

// Installer provides methods to manage the lifecycle of the custom resource definition.
type Installer struct {
	cli        typedv1.CustomResourceDefinitionInterface
	k8sVersion *utilversion.Version
}

func NewInstaller(c *rest.Config) (*Installer, error) {
	cli, err := crd.NewCRDClientFromConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init crd client")
	}

	k8sVersion, err := detectServerVersion(c)
	if err != nil {
		// Keep running; fallback behavior strips selectable fields on unknown versions.
		k8sVersion = nil
	}

	return &Installer{
		cli:        cli,
		k8sVersion: k8sVersion,
	}, nil
}

func (i *Installer) makeCRDVersionSafe(res *v1.CustomResourceDefinition) *v1.CustomResourceDefinition {
	res = ensureSelectableFieldsVersionSafe(res, i.k8sVersion)
	return res
}

func ensureSelectableFieldsVersionSafe(res *v1.CustomResourceDefinition, k8sVersion *utilversion.Version) *v1.CustomResourceDefinition {
	if k8sVersion != nil && k8sVersion.AtLeast(utilversion.MustParseGeneric("1.31.0")) {
		return res
	}

	copy := res.DeepCopy()
	for idx := range copy.Spec.Versions {
		copy.Spec.Versions[idx].SelectableFields = nil
	}

	return copy
}

func detectServerVersion(c *rest.Config) (*utilversion.Version, error) {
	disco, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init discovery client")
	}

	serverVersion, err := disco.ServerVersion()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get server version")
	}

	parsedK8sVersion, err := utilversion.ParseGeneric(serverVersion.GitVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse server version")
	}

	return parsedK8sVersion, nil
}

func (i *Installer) create(ctx context.Context, res *v1.CustomResourceDefinition) (*v1.CustomResourceDefinition, error) {
	res, err := i.cli.Create(ctx, i.makeCRDVersionSafe(res), metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create crd")
	}
	return res, nil
}

func (i *Installer) update(ctx context.Context, res *v1.CustomResourceDefinition) (*v1.CustomResourceDefinition, error) {
	updated, err := i.cli.Update(ctx, i.makeCRDVersionSafe(res), metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update existing crd")
	}

	return updated, nil
}

// Installs the embedded MultitenantPodNetworkConfig CRD definition in the cluster.
func (i *Installer) InstallMTPNC(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	mtpnc, err := GetMultitenantPodNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded mtpnc crd")
	}
	return i.create(ctx, mtpnc)
}

// InstallOrUpdate installs the embedded MultitenantPodNetworkConfig CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdateMTPNC(ctx context.Context) (*v1.CustomResourceDefinition, error) {
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
		current, err = i.update(ctx, mtpnc)
		if err != nil {
			return &previous, err
		}
	}
	return current, nil
}

// Install installs the embedded NodeInfo CRD definition in the cluster.
func (i *Installer) InstallNodeInfo(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nodeinfo, err := GetNodeInfo()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded nodeinfo crd")
	}
	return i.create(ctx, nodeinfo)
}

// InstallOrUpdate installs the embedded NodeInfo CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdateNodeInfo(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nodeinfo, err := GetNodeInfo()
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
		current, err = i.update(ctx, nodeinfo)
		if err != nil {
			return &previous, err
		}
	}
	return current, nil
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
		current, err = i.update(ctx, podNetwork)
		if err != nil {
			return &previous, err
		}
	}
	return current, nil
}

// Install installs the embedded PodNetworkInstance CRD definition in the cluster.
func (i *Installer) InstallPodNetworkInstance(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	podnetworkinstance, err := GetPodNetworkInstances()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded podnetworkinstance crd")
	}
	return i.create(ctx, podnetworkinstance)
}

// InstallOrUpdate installs the embedded PodNetworkInstance CRD definition in the cluster or updates it if present.
func (i *Installer) InstallOrUpdatePodNetworkInstance(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	podnetworkinstance, err := GetPodNetworkInstances()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded podnetworkinstance crd")
	}
	current, err := i.create(ctx, podnetworkinstance)
	if !apierrors.IsAlreadyExists(err) {
		return current, err
	}
	if current == nil {
		current, err = i.cli.Get(ctx, podnetworkinstance.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing podnetworkinstance crd")
		}
	}
	if !reflect.DeepEqual(podnetworkinstance.Spec.Versions, current.Spec.Versions) {
		podnetworkinstance.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = i.update(ctx, podnetworkinstance)
		if err != nil {
			return &previous, err
		}
	}
	return current, nil
}

type NodeInfoClient struct {
	Cli client.Client
}

func (n *NodeInfoClient) CreateOrUpdate(ctx context.Context, nodeInfo *v1alpha1.NodeInfo, fieldOwner string) error {
	if err := n.Cli.Create(ctx, nodeInfo); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "error creating nodeinfo crd")
		}
		if err := n.Cli.Patch(ctx, &v1alpha1.NodeInfo{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "NodeInfo",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeInfo.Name,
			},
			Spec: nodeInfo.Spec,
		}, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
			return errors.Wrap(err, "error patching nodeinfo crd")
		}
	}
	return nil
}

// Get retrieves the NodeInfo CRD by name.
func (n *NodeInfoClient) Get(ctx context.Context, name string) (*v1alpha1.NodeInfo, error) {
	var nodeInfo v1alpha1.NodeInfo
	if err := n.Cli.Get(ctx, client.ObjectKey{Name: name}, &nodeInfo); err != nil {
		return nil, errors.Wrap(err, "error getting nodeinfo crd")
	}
	return &nodeInfo, nil
}
