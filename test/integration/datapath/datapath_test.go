package datapath

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/internal/k8sutils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

var (
	restConfig *restclient.Config
	clientset  *kubernetes.Clientset
	testConfig = &TestConfig{}
	// Set a env variable for below
	podCount     = 2
	podPrefix    = "datapod"
	podNamespace = "datapath-win" // or something datapath related
	podLabelKey  = "app"
	// podLabelSelector   = "app=datapod"
	podLabelSelector   = fmt.Sprintf("%s=%s", podLabelKey, podPrefix)
	nodeLabelSelector  = "agentpool=npwina"
	WindowsPodYamlPath = "../manifests/datapath/windowspod.yaml"
)

const (
	defaultAttempts          = 15
	defaultRetryDelaySeconds = 1
	maxRetryDelaySeconds     = 10
)

/*
	var defaultRetrier = retry.Retrier{
		Attempts:   defaultAttempts,
		Delay:      defaultRetryDelaySeconds * time.Second,
		ExpBackoff: true,
		MaxDelay:   maxRetryDelaySeconds * time.Second,
	} // dncDeployedSuccessfully = false

// dncPortForward          = k8s.PortForwardStreamHandle{}
// dncClient               = &client.DncClient{}
// dncRetrier              = DNCRetrier{}
// rbacCleanUpFn = func(t *testing.T) {}
*/
type TestConfig struct {
	/*
		CNIManagerImage             string `env:"CNI_MANAGER_IMAGE"`
		CNSImage                    string `env:"CNS_IMAGE"`
		DNCImage                    string `env:"DNC_IMAGE"`
		CNIManagerDaemonsetYamlPath string `env:"CNI_MANAGER_DAEMONSET_YAML_PATH"`
		CNSDaemonsetYamlPath        string `env:"CNS_DAEMONSET_YAML_PATH"`
		CNSWindowsDaemonsetYamlPath string `env:"CNS_WINDOWS_DAEMONSET_YAML_PATH"`
		CNSConfigmapYamlPath        string `env:"CNS_CONFIGMAP_YAML_PATH"`
		DNCDeploymentYamlPath       string `env:"DNC_DEPLOYMENT_YAML_PATH"`
		DNCConfigmapYamlPath        string `env:"DNC_CONFIGMAP_YAML_PATH"`
		GoldpingerPodYamlPath       string `env:"GOLDPINGER_POD_YAML_PATH"`
		HostDaemonsetYamlPath       string `env:"HOST_DAEMONSET_YAML_PATH"`
		CosmosDbAccountName         string `env:"COSMOS_DB_ACCOUNT_NAME"`
		CosmosDbResourceId          string `env:"COSMOS_DB_RESOURCE_ID"`
		PartitionKey                string `env:"PARTITION_KEY"`
		DesiredNCsPerNode           int    `env:"DESIRED_NCS_PER_NODE"` // we only need 2, dont need any interaction from user
		InfraVnetGuid               string `env:"INFRA_VNET_GUID"`


	*/
	//Need to set yaml path to better dir
	// Current is /test/standalone/common/yamls/windows_deployment_pod.yaml

	// DNCRetrier Vars
	/*
		CustomerVnetGuid   string `env:"CUSTOMER_VNET_GUID"`
		CustomerSubnetName string `env:"CUSTOMER_SUBNET_NAME"`
		DelegationToken    string `env:"DELEGATION_TOKEN"`
	*/
}

// Clientset
// restconfig
// Pod deployment in correct nodes
// check that nodes a running/ready
func TestMain(m *testing.M) {
	// Dont need everything from testConfig
	// Running env variables in local go test
	// .. is very flaky. Changing... Hopefully
	var err error

	// Gets cluster context
	// restConfig := MustGetRestConfig()
	// logrus.Infof("k8s rest config for apiserver %s", restConfig.Host)

	// Creates a new ClientSet
	if clientset, err = k8sutils.MustGetClientset(); err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err)
	}

	osExitCode := m.Run()

	os.Exit(osExitCode)
}

func TestDatapathWin(t *testing.T) {
	// Give these vars a env/flag
	testTimeout := 30 * time.Minute
	var err error

	// rest config
	t.Log("REST config")
	restConfig := k8sutils.MustGetRestConfig(t)

	// logrus.Infof("k8s rest config for apiserver %s", restConfig.Host)

	// continue
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	// Pod Namespace
	t.Log("Create Namespace")
	/*
		err = k8sutils.MustCreateNamespace(ctx, clientset, podNamespace)
		if err != nil {
			t.Fatal(err)
		}
	*/
	_, err = clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: podNamespace,
		},
	}, metav1.CreateOptions{})

	createPodFlag := apierrors.IsAlreadyExists(err)

	// Pod Interface Getter
	t.Log("Pod Interface")
	podI := clientset.CoreV1().Pods(podNamespace)

	// Get NodeList WindowsNodePoolName
	t.Log("Get Nodes")
	nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
	if err != nil {
		logrus.Fatalf("could not get k8s node list: %v", err)
	}

	// Checks namespace already exists from previous test
	if createPodFlag {
		t.Log("Pod namespace already exists")
	} else {
		t.Log("Creating Windows pods")
		podCounter := 0
		for _, node := range nodes.Items {
			for i := 0; i < podCount; i++ {
				pod, err := k8sutils.MustParsePod(WindowsPodYamlPath)
				require.NoError(t, err, "Parsing windows pod deployment failed")
				pod.Spec.NodeSelector = make(map[string]string)
				pod.Spec.NodeSelector["kubernetes.io/hostname"] = node.ObjectMeta.Name
				pod.Name = fmt.Sprintf("%s-%d", podPrefix, podCounter)
				pod.Namespace = podNamespace
				pod.Labels = make(map[string]string)
				pod.Labels[podLabelKey] = podPrefix
				err = k8sutils.MustCreateOrUpdatePod(ctx, podI, pod)
				require.NoError(t, err, "Creating windows pods failed")
				err = k8sutils.WaitForPodsRunning(ctx, clientset, pod.Namespace, podLabelSelector)
				require.NoError(t, err, fmt.Sprintf("Deploying windows Pod: %s failed with error: %v", pod.Name, err))
				t.Logf("Successfully deployed windows pod: %s", pod.Name)
				podCounter += 1
			}
		}
		t.Log("Successfully created customer windows pods")
	}
	// logrus.Printf("nodes are %+v", nodes)
	// assign pods to each node with labels

	t.Run("Windows ping tests pod -> pod", func(t *testing.T) {
		// Get NodeList WindowsNodePoolName
		nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
		if err != nil {
			logrus.Fatalf("could not get k8s node list: %v", err)
		}

		// Windows pod ping tests
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				// Pod to pod same node
				// windowsPodToPodPingTestSameNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, labelSelector string, rc *restclient.Config)
				t.Log("Windows ping tests (1) - Same Node")
				err := windowsPodToPodPingTestSameNode(ctx, clientset, node.Name, podLabelSelector, restConfig)
				require.NoError(t, err, "Windows pod to pod, same node, ping test failed with %+v", err)
				t.Logf("Windows pod to windows pod, same node, passed for node: %s", node.ObjectMeta.Name)
			}
		}

		// Pod to pod different node
		for i := 0; i < len(nodes.Items); i++ {
			t.Log("Windows ping tests (1) - Different Node")
			firstNode := nodes.Items[i%2].Name
			secondNode := nodes.Items[(i+1)%2].Name
			err = windowsPodToPodPingTestDiffNode(ctx, clientset, firstNode, secondNode, podLabelSelector, restConfig)
			require.NoError(t, err, "Windows pod to pod, different node, ping test failed with %+v", err)
			t.Logf("Windows pod to windows pod, different node, passed for node: %s -> %s", firstNode, secondNode)

		}
	})

	t.Run("Windows ping tests pod -> node", func(t *testing.T) {
		// Windows ping tests between pods and node
		t.Log("Windows Pod to Host Ping tests")
		for _, node := range nodes.Items {
			t.Log("Windows ping tests (2)")
			// func windowsPodToNode(ctx context.Context, nodeName string, nodeIP string, labelSelector string, rc *restclient.Config) error {
			nodeIP := ""
			for _, address := range node.Status.Addresses {
				if address.Type == "InternalIP" {
					nodeIP = address.Address
					break
				}
			}

			err := windowsPodToNode(ctx, node.Name, nodeIP, podLabelSelector, restConfig)
			require.NoError(t, err, "Windows pod to node, ping test failed with %+v", err)
			t.Logf("Windows pod to node, passed for node: %s", node.Name)
		}
	})

	t.Run("Windows url tests pod -> internet", func(t *testing.T) {
		// From windows pod to curl an URL
		t.Log("Windows Pod to Internet tests")
		nodes, err := k8sutils.GetNodeListByLabelSelector(ctx, clientset, nodeLabelSelector)
		if err != nil {
			logrus.Fatalf("could not get k8s node list: %v", err)
		}
		for _, node := range nodes.Items {
			if node.Status.NodeInfo.OperatingSystem == string(apiv1.Windows) {
				err := windowsPodToInternet(ctx, clientset, node.Name, podLabelSelector, restConfig)
				require.NoError(t, err, "Windows pod to internet url %+v", err)
				t.Logf("Windows pod to Internet url tests")

				/*
					pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, podLabelSelector, node.Name)
					if err != nil {
						logrus.Fatalf("could not get k8s clientset: %v", err) // Rename log
					}

					for _, pod := range pods.Items {
						//func windowsPodToInternet(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, labelSelector string, rc *restclient.Config) error {
						err := windowsPodToInternet(ctx, clientset, node.Name, podLabelSelector, restConfig)
						require.NoError(t, err, "Windows pod to internet %+v", err)
						t.Logf("Windows pod to Internet tests")
					}
				*/

			}
		}
	})
}

func MustGetRestConfig() *restclient.Config {
	pwd, err := os.Getwd()
	fmt.Printf("%s envar\n", pwd)
	temp2, err := os.UserHomeDir()
	fmt.Printf("%s homeDir\n", temp2)
	// change directory from src/dnc to home/.kube
	dir := os.Chdir(temp2)
	fmt.Printf("%s envar\n", dir)

	pwd, _ = os.Getwd()
	fmt.Printf("%s envar\n", pwd)
	// cd home/jpayne/.kube
	// set var to kubeconfig filepath
	temp, err := os.Getwd()
	temp += "/config"
	fmt.Printf("%s envar\n", temp)
	os.Chdir(pwd)
	config, err := clientcmd.BuildConfigFromFlags("", temp)
	if err != nil {
		logrus.Fatalf("Buildconfig Error: %v", err)
	}
	return config
}

func podTest(ctx context.Context, clientset *kubernetes.Clientset, srcPod *apiv1.Pod, cmd []string, rc *restclient.Config, passFunc func(string) error) error {
	logrus.Infof("podTest() - %v %v", srcPod.Name, cmd)
	output, err := k8sutils.ExecCmdOnPod(ctx, clientset, srcPod.Namespace, srcPod.Name, cmd, rc)
	// output, err := k8sShim.ExecPodsByLabelSelector(ctx, srcPod.Namespace, fmt.Sprintf("app=%s", srcPod.Name), srcPod.Spec.Containers[0].Name, cmd, rc)
	if err != nil {
		return err
	}
	return passFunc(string(output))
}

// logrus.Infof("could not get k8s clientset: %v", err)
func windowsPodToPodPingTestSameNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, labelSelector string, rc *restclient.Config) error {
	logrus.Info("P2P Ping Test - Get Pods by Node")
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err) // Rename log
	}
	if len(pods.Items) < 2 {
		return fmt.Errorf("Only %d pods on node %s, requires at least 2 pods", len(pods.Items), nodeName)
	}
	podMap := make(map[int]string)
	for i, pod := range pods.Items {
		podMap[i] = pod.Name
	}

	// Get first pod on this node
	// firstPod, err := k8sShim.GetPod(ctx, node.allocatedNCs[0].PodNamespace, node.allocatedNCs[0].PodName)

	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, podMap[0], metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", podMap[0], err))
	}
	logrus.Infof("P2P Ping Test - First pod: %v", firstPod.Name)

	// Get the second pod on this node
	// secondPod, err := k8sShim.GetPod(ctx, node.allocatedNCs[1].PodNamespace, node.allocatedNCs[1].PodName)
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, podMap[1], metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", podMap[1], err))
	}
	logrus.Infof("P2P Ping Test - Second pod: %v", secondPod.Name)

	// Ping the second pod from the first pod
	return podTest(ctx, clientset, firstPod, []string{"ping", secondPod.Status.PodIP}, rc, pingPassedWindows)
}

func windowsPodToPodPingTestDiffNode(ctx context.Context, clientset *kubernetes.Clientset, nodeName1 string, nodeName2 string, labelSelector string, rc *restclient.Config) error {
	logrus.Info("P2P Ping Test - Get Pods by Node")
	// Node 1
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName1)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err) // Rename log
	}
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("P2P Ping Test - First pod: %v", firstPod.Name)

	// Node 2
	pods, err = k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName2)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err) // Rename log
	}
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("P2P Ping Test - Second pod: %v", secondPod.Name)

	return podTest(ctx, clientset, firstPod, []string{"ping", secondPod.Status.PodIP}, rc, pingPassedWindows)
}

func windowsPodToNode(ctx context.Context, nodeName string, nodeIP string, labelSelector string, rc *restclient.Config) error {
	logrus.Info("P2N Ping Test - Get Pods by Node")
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err) // Rename log
	}
	if len(pods.Items) < 2 {
		return fmt.Errorf("Only %d pods on node %s, requires at least 2 pods", len(pods.Items), nodeName)
	}
	// Get first pod on this node
	// firstPod, err := k8sShim.GetPod(ctx, node.allocatedNCs[0].PodNamespace, node.allocatedNCs[0].PodName)
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("P2N Ping Test - First pod: %v", firstPod.Name)

	// Get the second pod on this node
	// secondPod, err := k8sShim.GetPod(ctx, node.allocatedNCs[1].PodNamespace, node.allocatedNCs[1].PodName)
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[1].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("P2N Ping Test - Second pod: %v", secondPod.Name)

	// ping from first and second pod to node
	resultOne := podTest(ctx, clientset, firstPod, []string{"ping", nodeIP}, rc, pingPassedWindows)
	resultTwo := podTest(ctx, clientset, secondPod, []string{"ping", nodeIP}, rc, pingPassedWindows)

	if resultOne != nil {
		return resultOne
	}

	if resultTwo != nil {
		return resultTwo
	}

	return nil
}

func windowsPodToInternet(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, labelSelector string, rc *restclient.Config) error {
	logrus.Info("P2Internet Url Test - Get Pods by Node")
	pods, err := k8sutils.GetPodsByNode(ctx, clientset, podNamespace, labelSelector, nodeName)
	if err != nil {
		logrus.Fatalf("could not get k8s clientset: %v", err) // Rename log
	}

	// Get first pod on this node
	// firstPod, err := k8sShim.GetPod(ctx, node.allocatedNCs[0].PodNamespace, node.allocatedNCs[0].PodName)
	firstPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[0].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", firstPod.Name, err))
	}
	logrus.Infof("P2Internet Url Test - First pod: %v", firstPod.Name)

	// Get the second pod on this node
	// secondPod, err := GetPod(ctx, node.allocatedNCs[1].PodNamespace, node.allocatedNCs[1].PodName)
	secondPod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, pods.Items[1].Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Getting pod %s failed with %v", secondPod.Name, err))
	}
	logrus.Infof("P2Internet Url Test - Second pod: %v", secondPod.Name)
	// Can use curl, but need to have a certain version of powershell. Calls IWR by reference so use IWR.
	resultOne := podTest(ctx, clientset, firstPod, []string{"powershell", "Invoke-WebRequest", "www.bing.com", "-UseBasicParsing"}, rc, invokeWebRequestPassedWindows)
	resultTwo := podTest(ctx, clientset, secondPod, []string{"powershell", "Invoke-WebRequest", "www.bing.com", "-UseBasicParsing"}, rc, invokeWebRequestPassedWindows)
	// resultTwo := podTest(ctx, clientset, secondPod, []string{"Invoke-RestMethod -Uri", "bing.com"}, rc, invokeWebRequestPassedWindows)

	if resultOne != nil {
		return resultOne
	}

	if resultTwo != nil {
		return resultTwo
	}

	return nil
}

func invokeWebRequestPassedWindows(output string) error {
	const searchString = "200 OK"
	if strings.Contains(output, searchString) {
		return nil
	}
	return fmt.Errorf("Output did not contain \"%s\", considered failed, output was: %s", searchString, output)
}

func pingPassedWindows(output string) error {
	const searchString = "0% loss"
	if strings.Contains(output, searchString) {
		return nil
	}
	return fmt.Errorf("Output did not contain \"%s\", considered failed, output was: %s", searchString, output)
}
