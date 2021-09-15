package nodenetworkconfig

import (
	"context"
	"reflect"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	typedv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type Client struct {
	crd typedv1.CustomResourceDefinitionInterface
}

func NewClientWithConfig(c *rest.Config) (*Client, error) {
	crdCli, err := crd.NewCRDClient(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init nnc client")
	}
	return &Client{
		crd: crdCli,
	}, nil
}

func (c *Client) create(ctx context.Context, res *v1.CustomResourceDefinition) (*v1.CustomResourceDefinition, error) {
	res, err := c.crd.Create(ctx, res, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create nnc crd")
	}
	return res, nil
}

// Install installs the embedded NodeNetworkConfig CRD definition in the cluster.
func (c *Client) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nnc, err := GetNodeNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded nnc crd")
	}
	return c.create(ctx, nnc)
}

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
		current, err = c.crd.Get(ctx, nnc.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get existing nnc crd")
		}
	}
	if !reflect.DeepEqual(nnc.Spec.Versions, current.Spec.Versions) {
		nnc.SetResourceVersion(current.GetResourceVersion())
		previous := *current
		current, err = c.crd.Update(ctx, nnc, metav1.UpdateOptions{})
		if err != nil {
			return &previous, errors.Wrap(err, "failed to update existing nnc crd")
		}
	}
	return current, nil
}
