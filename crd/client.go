package crd

import (
	"github.com/pkg/errors"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	v1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	"k8s.io/client-go/rest"
)

func NewCRDClient(config *rest.Config) (v1.CustomResourceDefinitionInterface, error) {
	c, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init CRD client")
	}
	return c.ApiextensionsV1().CustomResourceDefinitions(), nil
}
