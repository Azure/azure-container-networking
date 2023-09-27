//go:build load

package load

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/validate"
	"github.com/stretchr/testify/require"
)

type TestConfig struct {
	OSType            string `env:"OS_TYPE" default:"linux"`
	CNIType           string `env:"CNI_TYPE" default:"cilium"`
	Iterations        int    `env:"ITERATIONS" default:"2"`
	ScaleUpReplicas   int    `env:"SCALE_UP" default:"10"`
	ScaleDownReplicas int    `env:"SCALE_DOWN" default:"1"`
	Replicas          int    `env:"REPLICAS" default:"1"`
	ValidateStateFile bool   `env:"VALIDATE_STATEFILE" default:"false"`
	ValidateDualStack bool   `env:"VALIDATE_DUALSTACK" default:"false"`
	ValidateV4Overlay bool   `env:"VALIDATE_V4OVERLAY" default:"false"`
	SkipWait          bool   `env:"SKIP_WAIT" default:"false"`
	RestartCase       bool   `env:"RESTART_CASE" default:"false"`
	Cleanup           bool   `env:"CLEANUP" default:"false"`
}

const (
	manifestDir      = "../manifests"
	podLabelSelector = "load-test=true"
	namespace        = "load-test"
)

var testConfig = &TestConfig{}

var noopDeploymentMap = map[string]string{
	"windows": manifestDir + "/noop-deployment-windows.yaml",
	"linux":   manifestDir + "/noop-deployment-linux.yaml",
}

/*
In order to run the scale tests, you need a k8s cluster and its kubeconfig.
If no kubeconfig is passed, the test will attempt to find one in the default location for kubectl config.
Run the tests as follows:

go test -timeout 30m -tags load -run ^TestLoad$

The Load test scale the pods up/down on the cluster and validates the pods have IP. By default it runs the
cycle for 2 iterations.

To validate the state file, set the flag -validate-statefile to true. By default it is set to false.
todo: consider adding the following scenarios
- [x] All pods should be assigned an IP.
- [x] Test the CNS state file.
- [x] Test the CNS Local cache.
- [x] Test the Cilium state file.
- [x] Test the Node restart.
- [x] Test based on operating system.
- [x] Test the HNS state file.
- [x] Parameterize the os, cni and number of iterations.
- [x] Add deployment yaml for windows.
*/
func TestLoad(t *testing.T) {
	clientset, err := kubernetes.MustGetClientset()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Create namespace if it doesn't exist
	namespaceExists, err := kubernetes.NamespaceExists(ctx, clientset, namespace)
	require.NoError(t, err)

	if !namespaceExists {
		err = kubernetes.MustCreateNamespace(ctx, clientset, namespace)
		require.NoError(t, err)
	}

	deployment, err := kubernetes.MustParseDeployment(noopDeploymentMap[testConfig.OSType])
	require.NoError(t, err)

	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	err = kubernetes.MustCreateDeployment(ctx, deploymentsClient, deployment)
	require.NoError(t, err)

	t.Log("Checking pods are running")
	err = kubernetes.WaitForPodsRunning(ctx, clientset, namespace, podLabelSelector)
	require.NoError(t, err)

	t.Log("Repeating the scale up/down cycle")
	for i := 0; i < testConfig.Iterations; i++ {
		t.Log("Iteration ", i)
		t.Log("Scale down deployment")
		err = kubernetes.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, testConfig.ScaleDownReplicas, testConfig.SkipWait)
		require.NoError(t, err)

		t.Log("Scale up deployment")
		err = kubernetes.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, testConfig.ScaleUpReplicas, testConfig.SkipWait)
		require.NoError(t, err)
	}
	t.Log("Checking pods are running and IP assigned")
	err = kubernetes.WaitForPodsRunning(ctx, clientset, "", "")
	require.NoError(t, err)

	if testConfig.ValidateStateFile {
		t.Run("Validate state file", TestValidateState)
	}

	if testConfig.ValidateV4Overlay {
		t.Run("Validate v4overlay", TestV4OverlayProperties)
	}

	if testConfig.ValidateDualStack {
		t.Run("Validate dualstack overlay", TestDualStackProperties)
	}

	if testConfig.Cleanup {
		err = kubernetes.MustDeleteDeployment(ctx, deploymentsClient, deployment)
		require.NoError(t, err, "error deleteing load deployment")
		err = kubernetes.WaitForPodsDelete(ctx, clientset, namespace, podLabelSelector)
		require.NoError(t, err, "error waiting for pods to delete")
	}
}

// TestValidateState validates the state file based on the os and cni type.
func TestValidateState(t *testing.T) {
	clientset, err := kubernetes.MustGetClientset()
	require.NoError(t, err)

	config := kubernetes.MustGetRestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	validator, err := validate.CreateValidator(ctx, clientset, config, namespace, testConfig.CNIType, testConfig.RestartCase, testConfig.OSType)
	require.NoError(t, err)

	err = validator.Validate(ctx)
	require.NoError(t, err)

	if testConfig.Cleanup {
		err = validator.Cleanup(ctx)
		require.NoError(t, err, "failed to cleanup validator")
	}
}

// TestScaleDeployment scales the deployment up/down based on the replicas passed.
// REPLICAS=10 go test -timeout 30m -tags load -run ^TestScaleDeployment$ -tags=load
func TestScaleDeployment(t *testing.T) {
	t.Log("Scale deployment")
	clientset, err := kubernetes.MustGetClientset()
	require.NoError(t, err)

	ctx := context.Background()
	// Create namespace if it doesn't exist
	namespaceExists, err := kubernetes.NamespaceExists(ctx, clientset, namespace)
	require.NoError(t, err)

	if !namespaceExists {
		err = kubernetes.MustCreateNamespace(ctx, clientset, namespace)
		require.NoError(t, err)
	}

	deployment, err := kubernetes.MustParseDeployment(noopDeploymentMap[testConfig.OSType])
	require.NoError(t, err)

	if testConfig.Cleanup {
		deploymentsClient := clientset.AppsV1().Deployments(namespace)
		err = kubernetes.MustCreateDeployment(ctx, deploymentsClient, deployment)
		require.NoError(t, err)
	}

	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	err = kubernetes.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, testConfig.Replicas, testConfig.SkipWait)
	require.NoError(t, err)

	if testConfig.Cleanup {
		err = kubernetes.MustDeleteDeployment(ctx, deploymentsClient, deployment)
		require.NoError(t, err, "error deleteing load deployment")
		err = kubernetes.WaitForPodsDelete(ctx, clientset, namespace, podLabelSelector)
		require.NoError(t, err, "error waiting for pods to delete")
	}
}

// TestValidCNSStateDuringScaleAndCNSRestartToTriggerDropgzInstall
// tests that dropgz install during a pod scaling event, does not crash cns
func TestValidCNSStateDuringScaleAndCNSRestartToTriggerDropgzInstall(t *testing.T) {
	clientset, err := kubernetes.MustGetClientset()
	require.NoError(t, err)

	config := kubernetes.MustGetRestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	validator, err := validate.CreateValidator(ctx, clientset, config, namespace, testConfig.CNIType, testConfig.RestartCase, testConfig.OSType)
	require.NoError(t, err)

	err = validator.Validate(ctx)
	require.NoError(t, err)

	deployment, err := kubernetes.MustParseDeployment(noopDeploymentMap[testConfig.OSType])
	require.NoError(t, err)
	deploymentsClient := clientset.AppsV1().Deployments(namespace)

	if testConfig.Cleanup {
		// Create a deployment
		err = kubernetes.MustCreateDeployment(ctx, deploymentsClient, deployment)
		require.NoError(t, err)
	}

	// Scale it up and "skipWait", so CNS restart can happen immediately after scale call is made (while pods are still creating)
	skipWait := true
	err = kubernetes.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, testConfig.ScaleUpReplicas, skipWait)
	require.NoError(t, err)

	// restart linux CNS (linux, windows)
	err = kubernetes.RestartCNSDaemonset(ctx, clientset)
	require.NoError(t, err)

	// wait for pods to settle before checking cns state (otherwise, race between getting pods in creating state, and getting CNS state file)
	err = kubernetes.WaitForPodDeployment(ctx, clientset, namespace, deployment.Name, podLabelSelector, testConfig.ScaleUpReplicas)
	require.NoError(t, err)

	// Validate the CNS state
	err = validator.Validate(ctx)
	require.NoError(t, err)

	// Scale it down
	err = kubernetes.MustScaleDeployment(ctx, deploymentsClient, deployment, clientset, namespace, podLabelSelector, testConfig.ScaleDownReplicas, skipWait)
	require.NoError(t, err)

	// restart linux CNS (linux, windows)
	err = kubernetes.RestartCNSDaemonset(ctx, clientset)
	require.NoError(t, err)

	// wait for pods to settle before checking cns state (otherwise, race between getting pods in terminating state, and getting CNS state file)
	err = kubernetes.WaitForPodDeployment(ctx, clientset, namespace, deployment.Name, podLabelSelector, testConfig.ScaleDownReplicas)
	require.NoError(t, err)

	// Validate the CNS state
	err = validator.Validate(ctx)
	require.NoError(t, err)

	if testConfig.Cleanup {
		err = kubernetes.MustDeleteDeployment(ctx, deploymentsClient, deployment)
		require.NoError(t, err, "error deleteing load deployment")
		err = kubernetes.WaitForPodsDelete(ctx, clientset, namespace, podLabelSelector)
		require.NoError(t, err, "error waiting for pods to delete")
	}
}

func TestV4OverlayProperties(t *testing.T) {
	if !testConfig.ValidateV4Overlay {
		return
	}
	clientset, err := kubernetes.MustGetClientset()
	require.NoError(t, err)

	config := kubernetes.MustGetRestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	validator, err := validate.CreateValidator(ctx, clientset, config, namespace, testConfig.CNIType, testConfig.RestartCase, testConfig.OSType)
	require.NoError(t, err)

	// validate IPv4 overlay scenarios
	t.Log("Validating v4Overlay node labels")
	err = validator.ValidateV4OverlayControlPlane(ctx)
	require.NoError(t, err)
}

func TestDualStackProperties(t *testing.T) {
	if !testConfig.ValidateDualStack {
		return
	}
	clientset, err := kubernetes.MustGetClientset()
	require.NoError(t, err)

	config := kubernetes.MustGetRestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Log("Validating the dualstack node labels")
	validator, err := validate.CreateValidator(ctx, clientset, config, namespace, testConfig.CNIType, testConfig.RestartCase, testConfig.OSType)
	require.NoError(t, err)

	// validate dualstack overlay scenarios
	err = validator.ValidateDualStackControlPlane(ctx)
	require.NoError(t, err)

	if testConfig.Cleanup {
		err = validator.Cleanup(ctx)
		require.NoError(t, err, "failed to cleanup validator")
	}
}
