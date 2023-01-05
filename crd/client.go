package crd

import (
	"github.com/pkg/errors"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	v1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	"k8s.io/client-go/rest"
)

// NewCRDCLientFromConfig creates a CRD-scoped client from the provided kubeconfig.
func NewCRDClientFromConfig(config *rest.Config) (v1.CustomResourceDefinitionInterface, error) {
	c, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init CRD client")
	}
	return NewCRDClientFromClientset(c)
}

// NewCRDCLientFromConfig creates a CRD-scoped client from the provided kube clientset.
func NewCRDClientFromClientset(c *clientset.Clientset) (v1.CustomResourceDefinitionInterface, error) {
	return c.ApiextensionsV1().CustomResourceDefinitions(), nil
}
