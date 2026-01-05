package longrunningcluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/Azure/azure-container-networking/test/integration/swiftv2/helpers"
)

var (
	ErrNoLowNICNodes            = errors.New("no low-NIC nodes available")
	ErrNoHighNICNodes           = errors.New("no high-NIC nodes available")
	ErrAllLowNICNodesInUse      = errors.New("all low-NIC nodes already in use")
	ErrAllHighNICNodesInUse     = errors.New("all high-NIC nodes already in use")
	ErrFailedToGenerateSASToken = errors.New("failed to generate SAS token")
	ErrSASTokenEmpty            = errors.New("generated SAS token is empty")
	ErrSASTokenInvalid          = errors.New("generated SAS token appears invalid")
	ErrPodNotRunning            = errors.New("pod is not running")
	ErrHTTPAuthError            = errors.New("HTTP authentication error from private endpoint")
	ErrBlobNotFound             = errors.New("blob not found (404) on private endpoint")
	ErrUnexpectedBlobResponse   = errors.New("unexpected response from blob download (no 'Hello' or '200 OK' found)")
	ErrInvalidWorkloadType      = errors.New("invalid workload type")
	ErrUnexpectedTCPResponse    = errors.New("unexpected TCP response")
)

func getKubeconfigPath(clusterName string) string {
	kubeconfigDir := os.Getenv("KUBECONFIG_DIR")
	if kubeconfigDir == "" {
		kubeconfigDir = "/tmp"
	}
	return fmt.Sprintf("%s/%s.kubeconfig", kubeconfigDir, clusterName)
}

func applyTemplate(templatePath string, data interface{}, kubeconfig string) error {
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %w\nOutput: %s", err, string(out))
	}

	return nil
}

type PodNetworkData struct {
	PNName      string
	VnetGUID    string
	SubnetGUID  string
	SubnetARMID string
	SubnetToken string
}

func CreatePodNetwork(kubeconfig string, data PodNetworkData, templatePath string) error {
	return applyTemplate(templatePath, data, kubeconfig)
}

type PNIData struct {
	PNIName      string
	PNName       string
	Namespace    string
	Reservations int
}

func CreatePodNetworkInstance(kubeconfig string, data PNIData, templatePath string) error {
	return applyTemplate(templatePath, data, kubeconfig)
}

type PodData struct {
	PodName   string
	NodeName  string
	OS        string
	PNName    string
	PNIName   string
	Namespace string
	Image     string
}

func CreatePod(kubeconfig string, data PodData, templatePath string) error {
	return applyTemplate(templatePath, data, kubeconfig)
}

type TestResources struct {
	Kubeconfig         string
	PNName             string
	PNIName            string
	VnetGUID           string
	SubnetGUID         string
	SubnetARMID        string
	SubnetToken        string
	PodNetworkTemplate string
	PNITemplate        string
	PodTemplate        string
	PodImage           string
	Reservations       int
	Namespace          string
}

type PodScenario struct {
	Name          string // Descriptive name for the scenario
	Cluster       string // "aks-1" or "aks-2"
	VnetName      string // e.g., "cx_vnet_v1", "cx_vnet_v4"
	SubnetName    string // e.g., "s1", "s2"
	NodeSelector  string // "low-nic" or "high-nic"
	PodNameSuffix string // Unique suffix for pod name
}

type TestScenarios struct {
	ResourceGroup   string
	BuildID         string
	PodImage        string
	Scenarios       []PodScenario
	VnetSubnetCache map[string]VnetSubnetInfo
	UsedNodes       map[string]bool
}

type VnetSubnetInfo struct {
	VnetGUID    string
	SubnetGUID  string
	SubnetARMID string
	SubnetToken string
}

func isValidWorkloadType(workloadType string) bool {
	validTypes := []string{
		"swiftv2-linux",
		"swiftv2-windows",
		"swiftv2-linux-byon",
		"swiftv2-windows-byon",
	}

	for _, validType := range validTypes {
		if workloadType == validType {
			return true
		}
	}
	return false
}

type NodePoolInfo struct {
	LowNicNodes  []string
	HighNicNodes []string
}

func GetNodesByNicCount(kubeconfig string) (NodePoolInfo, error) {
	nodeInfo := NodePoolInfo{
		LowNicNodes:  []string{},
		HighNicNodes: []string{},
	}

	workloadType := strings.TrimSpace(os.Getenv("WORKLOAD_TYPE"))
	if workloadType == "" {
		workloadType = "swiftv2-linux"
	}

	if !isValidWorkloadType(workloadType) {
		return NodePoolInfo{}, fmt.Errorf("%w: %s", ErrInvalidWorkloadType, workloadType)
	}

	fmt.Printf("Filtering nodes by workload-type=%s\n", workloadType)

	lowNicLabelSelector := "nic-capacity=low-nic,workload-type=" + workloadType
	highNicLabelSelector := "nic-capacity=high-nic,workload-type=" + workloadType

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes",
		"-l", lowNicLabelSelector, "-o", "name")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return NodePoolInfo{}, fmt.Errorf("failed to get low-nic nodes: %w\nOutput: %s", err, string(out))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "node/") {
			nodeInfo.LowNicNodes = append(nodeInfo.LowNicNodes, strings.TrimPrefix(line, "node/"))
		}
	}

	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes",
		"-l", highNicLabelSelector, "-o", "name")
	out, err = cmd.CombinedOutput()
	if err != nil {
		return NodePoolInfo{}, fmt.Errorf("failed to get high-nic nodes: %w\nOutput: %s", err, string(out))
	}

	lines = strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line != "" && strings.HasPrefix(line, "node/") {
			nodeInfo.HighNicNodes = append(nodeInfo.HighNicNodes, strings.TrimPrefix(line, "node/"))
		}
	}

	fmt.Printf("Found %d low-nic nodes and %d high-nic nodes with workload-type=%s\n",
		len(nodeInfo.LowNicNodes), len(nodeInfo.HighNicNodes), workloadType)

	return nodeInfo, nil
}

func CreatePodNetworkResource(resources TestResources) error {
	err := CreatePodNetwork(resources.Kubeconfig, PodNetworkData{
		PNName:      resources.PNName,
		VnetGUID:    resources.VnetGUID,
		SubnetGUID:  resources.SubnetGUID,
		SubnetARMID: resources.SubnetARMID,
		SubnetToken: resources.SubnetToken,
	}, resources.PodNetworkTemplate)
	if err != nil {
		return fmt.Errorf("failed to create PodNetwork: %w", err)
	}
	return nil
}

func CreateNamespaceResource(kubeconfig, namespace string) error {
	err := helpers.EnsureNamespaceExists(kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	return nil
}

func CreatePodNetworkInstanceResource(resources TestResources) error {
	namespace := resources.Namespace
	if namespace == "" {
		namespace = resources.PNName
	}
	err := CreatePodNetworkInstance(resources.Kubeconfig, PNIData{
		PNIName:      resources.PNIName,
		PNName:       resources.PNName,
		Namespace:    namespace,
		Reservations: resources.Reservations,
	}, resources.PNITemplate)
	if err != nil {
		return fmt.Errorf("failed to create PodNetworkInstance: %w", err)
	}
	return nil
}

func CreatePodResource(resources TestResources, podName, nodeName string) error {
	err := CreatePod(resources.Kubeconfig, PodData{
		PodName:   podName,
		NodeName:  nodeName,
		OS:        "linux",
		PNName:    resources.PNName,
		PNIName:   resources.PNIName,
		Namespace: resources.PNName,
		Image:     resources.PodImage,
	}, resources.PodTemplate)
	if err != nil {
		return fmt.Errorf("failed to create pod %s: %w", podName, err)
	}

	err = helpers.WaitForPodRunning(resources.Kubeconfig, resources.PNName, podName, 10, 30)
	if err != nil {
		return fmt.Errorf("pod %s did not reach running state: %w", podName, err)
	}

	return nil
}

func GetOrFetchVnetSubnetInfo(rg, vnetName, subnetName string, cache map[string]VnetSubnetInfo) (VnetSubnetInfo, error) {
	key := fmt.Sprintf("%s/%s", vnetName, subnetName)

	if info, exists := cache[key]; exists {
		return info, nil
	}

	vnetGUID, err := helpers.GetVnetGUID(rg, vnetName)
	if err != nil {
		return VnetSubnetInfo{}, fmt.Errorf("failed to get VNet GUID: %w", err)
	}

	subnetGUID, err := helpers.GetSubnetGUID(rg, vnetName, subnetName)
	if err != nil {
		return VnetSubnetInfo{}, fmt.Errorf("failed to get Subnet GUID: %w", err)
	}

	subnetARMID, err := helpers.GetSubnetARMID(rg, vnetName, subnetName)
	if err != nil {
		return VnetSubnetInfo{}, fmt.Errorf("failed to get Subnet ARM ID: %w", err)
	}

	info := VnetSubnetInfo{
		VnetGUID:    vnetGUID,
		SubnetGUID:  subnetGUID,
		SubnetARMID: subnetARMID,
		SubnetToken: "",
	}

	cache[key] = info
	return info, nil
}

func CreateScenarioResources(scenario PodScenario, testScenarios TestScenarios) error {
	kubeconfig := getKubeconfigPath(scenario.Cluster)
	netInfo, err := GetOrFetchVnetSubnetInfo(testScenarios.ResourceGroup, scenario.VnetName, scenario.SubnetName, testScenarios.VnetSubnetCache)
	if err != nil {
		return fmt.Errorf("failed to get network info for %s/%s: %w", scenario.VnetName, scenario.SubnetName, err)
	}

	vnetShort := strings.TrimPrefix(scenario.VnetName, "cx_vnet_")
	vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
	subnetNameSafe := strings.ReplaceAll(scenario.SubnetName, "_", "-")
	pnName := fmt.Sprintf("pn-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)
	pniName := fmt.Sprintf("pni-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)

	resources := TestResources{
		Kubeconfig:         kubeconfig,
		PNName:             pnName,
		PNIName:            pniName,
		VnetGUID:           netInfo.VnetGUID,
		SubnetGUID:         netInfo.SubnetGUID,
		SubnetARMID:        netInfo.SubnetARMID,
		SubnetToken:        netInfo.SubnetToken,
		PodNetworkTemplate: "../../manifests/swiftv2/long-running-cluster/podnetwork.yaml",
		PNITemplate:        "../../manifests/swiftv2/long-running-cluster/podnetworkinstance.yaml",
		PodTemplate:        "../../manifests/swiftv2/long-running-cluster/pod.yaml",
		PodImage:           testScenarios.PodImage,
		Reservations:       2,
	}

	// Step 1: Create PodNetwork
	err = CreatePodNetworkResource(resources)
	if err != nil {
		return fmt.Errorf("scenario %s: %w", scenario.Name, err)
	}

	// Step 2: Create namespace
	err = CreateNamespaceResource(resources.Kubeconfig, resources.PNName)
	if err != nil {
		return fmt.Errorf("scenario %s: %w", scenario.Name, err)
	}

	// Step 3: Create PodNetworkInstance
	err = CreatePodNetworkInstanceResource(resources)
	if err != nil {
		return fmt.Errorf("scenario %s: %w", scenario.Name, err)
	}

	// Step 4: Get nodes by NIC count
	nodeInfo, err := GetNodesByNicCount(kubeconfig)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to get nodes: %w", scenario.Name, err)
	}

	// Step 5: Select appropriate node based on scenario
	var targetNode string

	if testScenarios.UsedNodes == nil {
		testScenarios.UsedNodes = make(map[string]bool)
	}

	if scenario.NodeSelector == "low-nic" {
		if len(nodeInfo.LowNicNodes) == 0 {
			return fmt.Errorf("%w: scenario %s", ErrNoLowNICNodes, scenario.Name)
		}
		targetNode = ""
		for _, node := range nodeInfo.LowNicNodes {
			if !testScenarios.UsedNodes[node] {
				targetNode = node
				testScenarios.UsedNodes[node] = true
				break
			}
		}
		if targetNode == "" {
			return fmt.Errorf("%w: scenario %s", ErrAllLowNICNodesInUse, scenario.Name)
		}
	} else {
		if len(nodeInfo.HighNicNodes) == 0 {
			return fmt.Errorf("%w: scenario %s", ErrNoHighNICNodes, scenario.Name)
		}
		targetNode = ""
		for _, node := range nodeInfo.HighNicNodes {
			if !testScenarios.UsedNodes[node] {
				targetNode = node
				testScenarios.UsedNodes[node] = true
				break
			}
		}
		if targetNode == "" {
			return fmt.Errorf("%w: scenario %s", ErrAllHighNICNodesInUse, scenario.Name)
		}
	}

	// Step 6: Create pod
	podName := "pod-" + scenario.PodNameSuffix
	err = CreatePodResource(resources, podName, targetNode)
	if err != nil {
		return fmt.Errorf("scenario %s: %w", scenario.Name, err)
	}

	fmt.Printf("Successfully created scenario: %s (pod: %s on node: %s)\n", scenario.Name, podName, targetNode)
	return nil
}

func DeleteScenarioResources(scenario PodScenario, buildID string) error {
	kubeconfig := getKubeconfigPath(scenario.Cluster)

	vnetShort := strings.TrimPrefix(scenario.VnetName, "cx_vnet_")
	vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
	subnetNameSafe := strings.ReplaceAll(scenario.SubnetName, "_", "-")
	pnName := fmt.Sprintf("pn-%s-%s-%s", buildID, vnetShort, subnetNameSafe)
	pniName := fmt.Sprintf("pni-%s-%s-%s", buildID, vnetShort, subnetNameSafe)
	podName := "pod-" + scenario.PodNameSuffix

	err := helpers.DeletePod(kubeconfig, pnName, podName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete pod: %w", scenario.Name, err)
	}

	err = helpers.DeletePodNetworkInstance(kubeconfig, pnName, pniName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete PNI: %w", scenario.Name, err)
	}

	err = helpers.DeletePodNetwork(kubeconfig, pnName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete PN: %w", scenario.Name, err)
	}

	err = helpers.DeleteNamespace(kubeconfig, pnName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete namespace: %w", scenario.Name, err)
	}

	fmt.Printf("Successfully deleted scenario: %s\n", scenario.Name)
	return nil
}

func CreateAllScenarios(testScenarios TestScenarios) error {
	for _, scenario := range testScenarios.Scenarios {
		fmt.Printf("\n=== Creating scenario: %s ===\n", scenario.Name)
		err := CreateScenarioResources(scenario, testScenarios)
		if err != nil {
			return err
		}
	}
	return nil
}

func DeleteAllScenarios(testScenarios TestScenarios) error {
	// Phase 1: Delete all pods first
	fmt.Printf("\n=== Phase 1: Deleting all pods ===\n")
	for _, scenario := range testScenarios.Scenarios {
		kubeconfig := getKubeconfigPath(scenario.Cluster)
		vnetShort := strings.TrimPrefix(scenario.VnetName, "cx_vnet_")
		vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
		subnetNameSafe := strings.ReplaceAll(scenario.SubnetName, "_", "-")
		pnName := fmt.Sprintf("pn-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)
		podName := "pod-" + scenario.PodNameSuffix

		fmt.Printf("Deleting pod for scenario: %s\n", scenario.Name)
		err := helpers.DeletePod(kubeconfig, pnName, podName)
		if err != nil {
			fmt.Printf("Warning: Failed to delete pod for scenario %s: %v\n", scenario.Name, err)
		}
	}

	// Phase 2: Delete shared PNI/PN/Namespace resources (grouped by vnet/subnet/cluster)
	fmt.Printf("\n=== Phase 2: Deleting shared PNI/PN/Namespace resources ===\n")
	resourceGroups := make(map[string]bool)

	for _, scenario := range testScenarios.Scenarios {
		kubeconfig := getKubeconfigPath(scenario.Cluster)
		vnetShort := strings.TrimPrefix(scenario.VnetName, "cx_vnet_")
		vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
		subnetNameSafe := strings.ReplaceAll(scenario.SubnetName, "_", "-")
		pnName := fmt.Sprintf("pn-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)
		pniName := fmt.Sprintf("pni-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)

		resourceKey := fmt.Sprintf("%s:%s", scenario.Cluster, pnName)
		if resourceGroups[resourceKey] {
			continue
		}
		resourceGroups[resourceKey] = true

		fmt.Printf("\nDeleting shared resources for %s/%s on %s\n", scenario.VnetName, scenario.SubnetName, scenario.Cluster)

		err := helpers.DeletePodNetworkInstance(kubeconfig, pnName, pniName)
		if err != nil {
			fmt.Printf("Warning: Failed to delete PNI %s: %v\n", pniName, err)
		}

		err = helpers.DeletePodNetwork(kubeconfig, pnName)
		if err != nil {
			fmt.Printf("Warning: Failed to delete PN %s: %v\n", pnName, err)
		}

		err = helpers.DeleteNamespace(kubeconfig, pnName)
		if err != nil {
			fmt.Printf("Warning: Failed to delete namespace %s: %v\n", pnName, err)
		}
	}

	// Phase 3: Verify no MTPNC resources are stuck
	fmt.Printf("\n=== Phase 3: Verifying MTPNC cleanup ===\n")
	clustersChecked := make(map[string]bool)

	for _, scenario := range testScenarios.Scenarios {
		if clustersChecked[scenario.Cluster] {
			continue
		}
		clustersChecked[scenario.Cluster] = true

		kubeconfig := getKubeconfigPath(scenario.Cluster)
		fmt.Printf("Checking for pending MTPNC resources in cluster %s\n", scenario.Cluster)

		err := helpers.VerifyNoMTPNC(kubeconfig, testScenarios.BuildID)
		if err != nil {
			fmt.Printf("WARNING: Found pending MTPNC resources in cluster %s: %v\n", scenario.Cluster, err)
		} else {
			fmt.Printf("No pending MTPNC resources found in cluster %s\n", scenario.Cluster)
		}
	}

	fmt.Printf("\n=== All scenarios deleted ===\n")
	return nil
}

func DeleteTestResources(kubeconfig, pnName, pniName string) error {
	for i := 0; i < 2; i++ {
		podName := fmt.Sprintf("pod-c2-%d", i)
		err := helpers.DeletePod(kubeconfig, pnName, podName)
		if err != nil {
			return fmt.Errorf("failed to delete pod %s: %w", podName, err)
		}
	}

	err := helpers.DeletePodNetworkInstance(kubeconfig, pnName, pniName)
	if err != nil {
		return fmt.Errorf("failed to delete PodNetworkInstance: %w", err)
	}

	err = helpers.DeletePodNetwork(kubeconfig, pnName)
	if err != nil {
		return fmt.Errorf("failed to delete PodNetwork: %w", err)
	}

	err = helpers.DeleteNamespace(kubeconfig, pnName)
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	return nil
}

// ConnectivityTest defines a connectivity test between two pods
type ConnectivityTest struct {
	Name            string
	SourcePod       string
	SourceNamespace string // Namespace of the source pod
	DestinationPod  string
	DestNamespace   string // Namespace of the destination pod
	Cluster         string // Cluster where source pod is running (for backward compatibility)
	DestCluster     string // Cluster where destination pod is running (if different from source)
	Description     string
	ShouldFail      bool // If true, connectivity is expected to fail (NSG block, customer isolation)

	// Fields for private endpoint tests
	SourceCluster string // Cluster where source pod is running
	SourcePodName string // Name of the source pod
	SourceNS      string // Namespace of the source pod
	DestEndpoint  string // Destination endpoint (IP or hostname)
	TestType      string // Type of test: "pod-to-pod" or "storage-access"
	Purpose       string // Description of the test purpose
}

// RunConnectivityTest tests TCP connectivity between two pods using netcat
func RunConnectivityTest(test ConnectivityTest) error {
	sourceKubeconfig := getKubeconfigPath(test.Cluster)

	destKubeconfig := sourceKubeconfig
	if test.DestCluster != "" {
		destKubeconfig = getKubeconfigPath(test.DestCluster)
	}

	destIP, err := helpers.GetPodDelegatedIP(destKubeconfig, test.DestNamespace, test.DestinationPod)
	if err != nil {
		return fmt.Errorf("failed to get destination pod delegated IP: %w", err)
	}

	fmt.Printf("Testing TCP connectivity from %s/%s (cluster: %s) to %s/%s (cluster: %s, eth1: %s) on port 8080\n",
		test.SourceNamespace, test.SourcePod, test.Cluster,
		test.DestNamespace, test.DestinationPod, test.DestCluster, destIP)

	// Use netcat to test TCP connectivity through the delegated subnet interface (eth1)
	// -w 3: 3 second timeout for connection
	// -z: Zero-I/O mode (scanning) - just check if port is open
	// Route through eth1 by binding to its IP address
	eth1IP, err := helpers.GetPodDelegatedIP(sourceKubeconfig, test.SourceNamespace, test.SourcePod)
	if err != nil {
		return fmt.Errorf("failed to get source pod eth1 IP: %w", err)
	}

	// Test TCP connection: send test message and read response
	ncCmd := fmt.Sprintf("echo 'test' | nc -w 3 -s %s %s 8080", eth1IP, destIP)

	output, err := helpers.ExecInPod(sourceKubeconfig, test.SourceNamespace, test.SourcePod, ncCmd)
	if err != nil {
		return fmt.Errorf("TCP connectivity test failed: %w\nOutput: %s", err, output)
	}

	// Verify we got the expected response from the TCP server
	if strings.Contains(output, "TCP Connection Success") {
		fmt.Printf("TCP connectivity successful! Response: %s\n", truncateString(output, 100))
		return nil
	}

	return fmt.Errorf("%w (expected 'TCP Connection Success')\nOutput: %s", ErrUnexpectedTCPResponse, truncateString(output, 100))
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func GenerateStorageSASToken(storageAccountName, containerName, blobName string) (string, error) {
	expiryTime := time.Now().UTC().Add(7 * 24 * time.Hour).Format("2006-01-02")

	cmd := exec.Command("az", "storage", "blob", "generate-sas",
		"--account-name", storageAccountName,
		"--container-name", containerName,
		"--name", blobName,
		"--permissions", "r",
		"--expiry", expiryTime,
		"--output", "tsv")

	out, err := cmd.CombinedOutput()
	sasToken := strings.TrimSpace(string(out))

	accountKeyWorked := err == nil && !strings.Contains(sasToken, "WARNING") &&
		!strings.Contains(sasToken, "ERROR") && (strings.Contains(sasToken, "sv=") || strings.Contains(sasToken, "sig="))

	if !accountKeyWorked {
		if err != nil {
			fmt.Printf("Account key SAS generation failed (error): %s\n", string(out))
		} else {
			fmt.Printf("Account key SAS generation failed (no credentials): %s\n", sasToken)
		}

		cmd = exec.Command("az", "storage", "blob", "generate-sas",
			"--account-name", storageAccountName,
			"--container-name", containerName,
			"--name", blobName,
			"--permissions", "r",
			"--expiry", expiryTime,
			"--auth-mode", "login",
			"--as-user",
			"--output", "tsv")

		out, err = cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("%w (both account key and user delegation): %w\n%s", ErrFailedToGenerateSASToken, err, string(out))
		}

		sasToken = strings.TrimSpace(string(out))
	}

	if sasToken == "" {
		return "", ErrSASTokenEmpty
	}
	sasToken = strings.Trim(sasToken, "\"'")
	if !strings.Contains(sasToken, "sv=") && !strings.Contains(sasToken, "sig=") {
		return "", fmt.Errorf("%w (missing sv= or sig=): %s", ErrSASTokenInvalid, sasToken)
	}

	return sasToken, nil
}

func GetStoragePrivateEndpoint(storageAccountName string) (string, error) {
	return storageAccountName + ".blob.core.windows.net", nil
}

func RunPrivateEndpointTest(test ConnectivityTest) error {
	kubeconfig := getKubeconfigPath(test.SourceCluster)

	fmt.Printf("Testing private endpoint access from %s to %s\n",
		test.SourcePodName, test.DestEndpoint)

	// Step 1: Verify pod is running
	fmt.Printf("==> Verifying pod %s is running\n", test.SourcePodName)
	podStatusCmd := fmt.Sprintf("kubectl --kubeconfig %s get pod %s -n %s -o jsonpath='{.status.phase}'", kubeconfig, test.SourcePodName, test.SourceNS)
	statusOut, err := exec.Command("sh", "-c", podStatusCmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get pod status: %w\nOutput: %s", err, string(statusOut))
	}
	podStatus := strings.TrimSpace(string(statusOut))
	if podStatus != "Running" {
		return fmt.Errorf("%w: pod %s (status: %s)", ErrPodNotRunning, test.SourcePodName, podStatus)
	}
	fmt.Printf("Pod is running\n")

	// Step 2: Verify DNS resolution with longer timeout
	fmt.Printf("==> Checking DNS resolution for %s\n", test.DestEndpoint)
	resolveCmd := fmt.Sprintf("nslookup %s | tail -2", test.DestEndpoint)
	resolveOutput, resolveErr := ExecInPodWithTimeout(kubeconfig, test.SourceNS, test.SourcePodName, resolveCmd, 20*time.Second)
	if resolveErr != nil {
		return fmt.Errorf("DNS resolution failed: %w\nOutput: %s", resolveErr, resolveOutput)
	}
	fmt.Printf("DNS Resolution Result:\n%s\n", resolveOutput)

	// Step 3: Generate SAS token for test blob
	fmt.Printf("==> Generating SAS token for test blob\n")
	// Extract storage account name from FQDN (e.g., sa106936191.blob.core.windows.net -> sa106936191)
	storageAccountName := strings.Split(test.DestEndpoint, ".")[0]
	sasToken, err := GenerateStorageSASToken(storageAccountName, "test", "hello.txt")
	if err != nil {
		return fmt.Errorf("failed to generate SAS token: %w", err)
	}

	// Step 4: Download test blob using SAS token with verbose output
	fmt.Printf("==> Downloading test blob via private endpoint\n")
	blobURL := fmt.Sprintf("https://%s/test/hello.txt?%s", test.DestEndpoint, sasToken)

	// Use wget instead of curl - it handles special characters better
	// -O- outputs to stdout, -q is quiet mode, --timeout sets timeout
	wgetCmd := fmt.Sprintf("wget -O- --timeout=30 --tries=1 '%s' 2>&1", blobURL)

	output, err := ExecInPodWithTimeout(kubeconfig, test.SourceNS, test.SourcePodName, wgetCmd, 45*time.Second)
	if err != nil {
		if strings.Contains(output, "ERROR 403") || strings.Contains(output, "ERROR 401") {
			return fmt.Errorf("%w\nOutput: %s", ErrHTTPAuthError, truncateString(output, 500))
		}
		if strings.Contains(output, "ERROR 404") {
			return fmt.Errorf("%w\nOutput: %s", ErrBlobNotFound, truncateString(output, 500))
		}
		return fmt.Errorf("private endpoint connectivity test failed: %w\nOutput: %s", err, truncateString(output, 500))
	}

	if strings.Contains(output, "Hello") || strings.Contains(output, "200 OK") || strings.Contains(output, "saved") {
		fmt.Printf("Private endpoint access successful!\n")
		return nil
	}

	return fmt.Errorf("%w\nOutput: %s", ErrUnexpectedBlobResponse, truncateString(output, 500))
}

func ExecInPodWithTimeout(kubeconfig, namespace, podName, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "exec", podName,
		"-n", namespace, "--", "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return string(out), fmt.Errorf("command timed out after %v in pod %s: %w", timeout, podName, ctx.Err())
		}
		return string(out), fmt.Errorf("failed to exec in pod %s in namespace %s: %w", podName, namespace, err)
	}

	return string(out), nil
}
