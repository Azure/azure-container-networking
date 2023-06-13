package validate

import (
	"context"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Validator struct {
	ctx         context.Context
	clientset   *kubernetes.Clientset
	config      *rest.Config
	namespace   string
	cni         string
	restartCase bool
}

// Todo: Add the validation for the data path function for the linux/windows client.
type IValidator interface {
	ValidateStateFile() error
	ValidateRestartNetwork() error
	// ValidateDataPath() error
}

type validatorClient interface {
	CreateClient(ctx context.Context, clienset *kubernetes.Clientset, config *rest.Config, namespace, cni string, restartCase bool) IValidator
}

// Func to get the type of validator client based on the Operating system.
func GetValidatorClient(os string) validatorClient {
	switch os {
	case "linux":
		return &LinuxClient{}
	case "windows":
		return &WindowsClient{}
	default:
		return nil
	}
}
