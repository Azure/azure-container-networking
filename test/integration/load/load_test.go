//go:build load

package load

import (
	"context"
	"flag"
	"testing"
	"time"

	k8sutils "github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/Azure/azure-container-networking/test/validate"
)

const (
	manifestDir      = "../manifests"
	noopdeployment   = manifestDir + "/load/noop-deployment.yaml"
	podLabelSelector = "load-test=true"
)

var (
	osType            = flag.String("os", "linux", "Operating system to run the test on")
	cniType           = flag.String("cni", "cilium", "CNI to run the test on")
	iterations        = flag.Int("iterations", 2, "Number of iterations to run the test for")
	scaleUpReplicas   = flag.Int("scaleup", 10, "Number of replicas to scale up to")
	scaleDownReplicas = flag.Int("scaledown", 1, "Number of replicas to scale down to")
	replicas          = flag.Int("replicas", 1, "Number of replicas to scale up/down to")
	validateStateFile = flag.Bool("validate-statefile", false, "Validate the state file")
	skipWait          = flag.Bool("skip-wait", false, "Skip waiting for pods to be ready")
	restartCase       = flag.Bool("restart-case", false, "In restart case, skip if we don't find state file")
	namespace         = "load-test"
)

/*
In order to run the scale tests, you need a k8s cluster and its kubeconfig.
If no kubeconfig is passed, the test will attempt to find one in the default location for kubectl config.
Run the tests as follows:

go test -timeout 30m -tags load -run ^TestLoad$ -tags=load

The Load test scale the pods up/down on the cluster and validates the pods have IP. By default it runs the
cycle for 2 iterations.

To validate the state file, set the flag -validate-statefile to true. By default it is set to false.
todo: consider adding the following scenarios
- [x] All pods should be assigned an IP.
- [x] Test the CNS state file.
- [x] Test the CNS Local cache.
- [x] Test the Cilium state file.
- [x] Test the Node restart.
- [ ] Test based on operating system.
- [ ] Test the HNS state file.
- [ ] Parameterize the os, cni and number of iterations.
- [ ] Add deployment yaml for windows.
*/
func TestLoad(t *testing.T) {
	clientset, err := k8sutils.MustGetClientset()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Create namespace if it doesn't exist
	namespaceExists, err := k8sutils.NamespaceExists(ctx, clientset, namespace)
	if err != nil {
		t.Fatal(err)
	}

	if !namespaceExists {
		err = k8sutils.MustCreateNamespace(ctx, clientset, namespace)
		if err != nil {
			t.Fatal(err)
		}
	}

	deployment, err := k8sutils.MustParseDeployment(noopdeployment)
	if err != nil {
		t.Fatal(err)
	}

	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	err = k8sutils.MustCreateDeployment(ctx, deploymentsClient, deployment)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Checking pods are running")
	err = k8sutils.WaitForPodsRunning(ctx, clientset, namespace, podLabelSelector)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Repeating the scale up/down cycle")
	for i := 0; i < *iterations; i++ {
		t.Log("Iteration ", i)
		t.Log("Scale down deployment")
		err = k8sutils.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, *scaleDownReplicas, *skipWait)
		if err != nil {
			t.Fatal(err)
		}
		t.Log("Scale up deployment")
		err = k8sutils.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, *scaleUpReplicas, *skipWait)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Log("Checking pods are running and IP assigned")
	err = k8sutils.WaitForPodsRunning(ctx, clientset, "", "")
	if err != nil {
		t.Fatal(err)
	}

	if *validateStateFile {
		t.Run("Validate state file", TestValidateState)
	}
}

// TestValidateState validates the state file based on the os and cni type.
func TestValidateState(t *testing.T) {
	clientset, err := k8sutils.MustGetClientset()
	if err != nil {
		t.Fatal(err)
	}
	config := k8sutils.MustGetRestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Log("Validating the state file")
	validatorClient := validate.GetValidatorClient(*osType)
	validator := validatorClient.CreateClient(ctx, clientset, config, namespace, *cniType, *restartCase)

	err = validator.ValidateStateFile()
	if err != nil {
		t.Fatal(err)
	}

	//We are restarting the systmemd network and checking that the connectivity works after the restart. For more details: https://github.com/cilium/cilium/issues/18706
	t.Log("Validating the restart network scenario")
	err = validator.ValidateRestartNetwork()
	if err != nil {
		t.Fatal(err)
	}
}

// TestScaleDeployment scales the deployment up/down based on the replicas passed.
// go test -timeout 30m -tags load -run ^TestScaleDeployment$ -tags=load -replicas 10
func TestScaleDeployment(t *testing.T) {
	t.Log("Scale deployment")
	clientset, err := k8sutils.MustGetClientset()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Create namespace if it doesn't exist
	namespaceExists, err := k8sutils.NamespaceExists(ctx, clientset, namespace)
	if err != nil {
		t.Fatal(err)
	}

	if !namespaceExists {
		err = k8sutils.MustCreateNamespace(ctx, clientset, namespace)
		if err != nil {
			t.Fatal(err)
		}
	}

	deployment, err := k8sutils.MustParseDeployment(noopdeployment)
	if err != nil {
		t.Fatal(err)
	}
	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	err = k8sutils.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, *replicas, *skipWait)
	if err != nil {
		t.Fatal(err)
	}
}
