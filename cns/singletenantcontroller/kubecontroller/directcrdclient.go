package kubecontroller

import (
	"context"

	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// Implements DirectCRDClient interface
var _ DirectCRDClient = &CRDDirectClient{}

// CRDDirectClient is a direct client to CRDs in the API Server.
type CRDDirectClient struct {
	restClient *rest.RESTClient
}

// Get gets a crd
func (crdClient *CRDDirectClient) Get(ctx context.Context, name, namespace, typeName string) (*nnc.NodeNetworkConfig, error) {
	var (
		nodeNetConfig *nnc.NodeNetworkConfig
		err           error
	)

	nodeNetConfig = &nnc.NodeNetworkConfig{}
	if err = crdClient.restClient.Get().Namespace(namespace).Resource(crdTypeName).Name(name).Do(ctx).Into(nodeNetConfig); err != nil {
		return nil, err
	}

	return nodeNetConfig, nil
}

// NewCRDDirectClient creates a new direct crd client to the api server
func NewCRDDirectClient(kubeconfig *rest.Config, groupVersion *schema.GroupVersion) (*CRDDirectClient, error) {
	var (
		config     rest.Config
		restClient *rest.RESTClient
		err        error
	)

	config = *kubeconfig
	config.GroupVersion = groupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()
	if restClient, err = rest.RESTClientFor(&config); err != nil {
		return nil, err
	}

	return &CRDDirectClient{
		restClient: restClient,
	}, nil
}
