package nodenetworkconfig

import (
	"context"

	"github.com/Azure/azure-container-networking/crd"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	typedv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
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

// Install installs the embedded NodeNetworkConfig CRD definition in the cluster.
func (c *Client) Install(ctx context.Context) (*v1.CustomResourceDefinition, error) {
	nnc, err := GetNodeNetworkConfigs()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get embedded nnc crd")
	}
	current, err := c.crd.Create(ctx, nnc, metav1.CreateOptions{})
	if err != nil {
		return current, errors.Wrap(err, "failed to install nnc crd")
	}
	return current, nil
}
