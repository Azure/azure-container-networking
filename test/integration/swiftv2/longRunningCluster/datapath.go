package longRunningCluster

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
	// ErrNoLowNICNodes is returned when no low-NIC nodes are available
	ErrNoLowNICNodes = errors.New("no low-NIC nodes available")
	// ErrNoHighNICNodes is returned when no high-NIC nodes are available
	ErrNoHighNICNodes = errors.New("no high-NIC nodes available")
	// ErrAllLowNICNodesInUse is returned when all low-NIC nodes are already in use
	ErrAllLowNICNodesInUse = errors.New("all low-NIC nodes already in use")
	// ErrAllHighNICNodesInUse is returned when all high-NIC nodes are already in use
	ErrAllHighNICNodesInUse = errors.New("all high-NIC nodes already in use")
	// ErrFailedToGenerateSASToken is returned when SAS token generation fails
	ErrFailedToGenerateSASToken = errors.New("failed to generate SAS token")
	// ErrSASTokenEmpty is returned when generated SAS token is empty
	ErrSASTokenEmpty = errors.New("generated SAS token is empty")
	// ErrSASTokenInvalid is returned when generated SAS token appears invalid
	ErrSASTokenInvalid = errors.New("generated SAS token appears invalid")
	// ErrPodNotRunning is returned when pod is not in running state
	ErrPodNotRunning = errors.New("pod is not running")
	// ErrHTTPAuthError is returned when HTTP authentication fails for private endpoint
	ErrHTTPAuthError = errors.New("HTTP authentication error from private endpoint")
	// ErrBlobNotFound is returned when blob is not found (404) on private endpoint
	ErrBlobNotFound = errors.New("blob not found (404) on private endpoint")
	// ErrUnexpectedBlobResponse is returned when blob download response is unexpected
	ErrUnexpectedBlobResponse = errors.New("unexpected response from blob download (no 'Hello' or '200 OK' found)")
	// ErrInvalidWorkloadType is returned when workload type is invalid
	ErrInvalidWorkloadType = errors.New("invalid workload type")
)

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

// -------------------------
// PodNetwork
// -------------------------
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

// -------------------------
// PodNetworkInstance
// -------------------------
type PNIData struct {
	PNIName      string
	PNName       string
	Namespace    string
	Type         string
	Reservations int
}

func CreatePodNetworkInstance(kubeconfig string, data PNIData, templatePath string) error {
	return applyTemplate(templatePath, data, kubeconfig)
}

// -------------------------
// Pod
// -------------------------
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

// -------------------------
// High-level orchestration
// -------------------------

// TestResources holds all the configuration needed for creating test resources
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
}

// PodScenario defines a single pod creation scenario
type PodScenario struct {
	Name          string // Descriptive name for the scenario
	Cluster       string // "aks-1" or "aks-2"
	VnetName      string // e.g., "cx_vnet_v1", "cx_vnet_v4"
	SubnetName    string // e.g., "s1", "s2"
	NodeSelector  string // "low-nic" or "high-nic"
	PodNameSuffix string // Unique suffix for pod name
}

// TestScenarios holds all pod scenarios to test
type TestScenarios struct {
	ResourceGroup   string
	BuildID         string
	PodImage        string
	Scenarios       []PodScenario
	VnetSubnetCache map[string]VnetSubnetInfo // Cache for vnet/subnet info
	UsedNodes       map[string]bool           // Tracks which nodes are already used (one pod per node for low-NIC)
}

// VnetSubnetInfo holds network information for a vnet/subnet combination
type VnetSubnetInfo struct {
	VnetGUID    string
	SubnetGUID  string
	SubnetARMID string
	SubnetToken string
}

// isValidWorkloadType validates workload type to prevent command injection
func isValidWorkloadType(workloadType string) bool {
	// Only allow alphanumeric, dash, and underscore characters
	for _, r := range workloadType {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return workloadType != "" && len(workloadType) <= 64
}

// NodePoolInfo holds information about nodes in different pools
type NodePoolInfo struct {
	LowNicNodes  []string
	HighNicNodes []string
}

// GetNodesByNicCount categorizes nodes by NIC count based on nic-capacity labels
func GetNodesByNicCount(kubeconfig string) (NodePoolInfo, error) {
	nodeInfo := NodePoolInfo{
		LowNicNodes:  []string{},
		HighNicNodes: []string{},
	}

	// Get workload type from environment variable (defaults to swiftv2-linux)
	workloadType := os.Getenv("WORKLOAD_TYPE")
	if workloadType == "" {
		workloadType = "swiftv2-linux"
	}

	// Validate workloadType to prevent command injection
	if !isValidWorkloadType(workloadType) {
		return NodePoolInfo{}, fmt.Errorf("%w: %s", ErrInvalidWorkloadType, workloadType)
	}

	fmt.Printf("Filtering nodes by workload-type=%s\n", workloadType)

	// Get nodes with low-nic capacity and matching workload-type
	//#nosec G204 -- workloadType is validated above
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes",
		"-l", "nic-capacity=low-nic,workload-type="+workloadType, "-o", "name")
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

	// Get nodes with high-nic capacity and matching workload-type
	//#nosec G204 -- workloadType is validated above
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "nodes",
		"-l", "nic-capacity=high-nic,workload-type="+workloadType, "-o", "name")
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

// CreatePodNetworkResource creates a PodNetwork
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

// CreateNamespaceResource creates a namespace
func CreateNamespaceResource(kubeconfig, namespace string) error {
	err := helpers.EnsureNamespaceExists(kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	return nil
}

// CreatePodNetworkInstanceResource creates a PodNetworkInstance
func CreatePodNetworkInstanceResource(resources TestResources) error {
	err := CreatePodNetworkInstance(resources.Kubeconfig, PNIData{
		PNIName:      resources.PNIName,
		PNName:       resources.PNName,
		Namespace:    resources.PNName,
		Type:         "explicit",
		Reservations: 2,
	}, resources.PNITemplate)
	if err != nil {
		return fmt.Errorf("failed to create PodNetworkInstance: %w", err)
	}
	return nil
}

// CreatePodResource creates a single pod on a specified node and waits for it to be running
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

	// Wait for pod to be running with retries
	err = helpers.WaitForPodRunning(resources.Kubeconfig, resources.PNName, podName, 10, 30)
	if err != nil {
		return fmt.Errorf("pod %s did not reach running state: %w", podName, err)
	}

	return nil
}

// GetOrFetchVnetSubnetInfo retrieves cached network info or fetches it from Azure
func GetOrFetchVnetSubnetInfo(rg, vnetName, subnetName string, cache map[string]VnetSubnetInfo) (VnetSubnetInfo, error) {
	key := fmt.Sprintf("%s/%s", vnetName, subnetName)

	if info, exists := cache[key]; exists {
		return info, nil
	}

	// Fetch from Azure
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
		SubnetToken: "", // Token can be fetched if needed
	}

	cache[key] = info
	return info, nil
}

// CreateScenarioResources creates all resources for a specific pod scenario
func CreateScenarioResources(scenario PodScenario, testScenarios TestScenarios) error {
	// Get kubeconfig for the cluster
	kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", scenario.Cluster)

	// Get network info
	netInfo, err := GetOrFetchVnetSubnetInfo(testScenarios.ResourceGroup, scenario.VnetName, scenario.SubnetName, testScenarios.VnetSubnetCache)
	if err != nil {
		return fmt.Errorf("failed to get network info for %s/%s: %w", scenario.VnetName, scenario.SubnetName, err)
	}

	// Create unique names for this scenario (simplify vnet name and make K8s compatible)
	// Remove "cx_vnet_" prefix and replace underscores with hyphens
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

	// Initialize used nodes tracker if not exists
	if testScenarios.UsedNodes == nil {
		testScenarios.UsedNodes = make(map[string]bool)
	}

	if scenario.NodeSelector == "low-nic" {
		if len(nodeInfo.LowNicNodes) == 0 {
			return fmt.Errorf("%w: scenario %s", ErrNoLowNICNodes, scenario.Name)
		}
		// Find first unused node in the pool (low-NIC nodes can only handle one pod)
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
	} else { // "high-nic"
		if len(nodeInfo.HighNicNodes) == 0 {
			return fmt.Errorf("%w: scenario %s", ErrNoHighNICNodes, scenario.Name)
		}
		// Find first unused node in the pool
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

// DeleteScenarioResources deletes all resources for a specific pod scenario
func DeleteScenarioResources(scenario PodScenario, buildID string) error {
	kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", scenario.Cluster)

	// Create same names as creation (simplify vnet name and make K8s compatible)
	// Remove "cx_vnet_" prefix and replace underscores with hyphens
	vnetShort := strings.TrimPrefix(scenario.VnetName, "cx_vnet_")
	vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
	subnetNameSafe := strings.ReplaceAll(scenario.SubnetName, "_", "-")
	pnName := fmt.Sprintf("pn-%s-%s-%s", buildID, vnetShort, subnetNameSafe)
	pniName := fmt.Sprintf("pni-%s-%s-%s", buildID, vnetShort, subnetNameSafe)
	podName := "pod-" + scenario.PodNameSuffix

	// Delete pod
	err := helpers.DeletePod(kubeconfig, pnName, podName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete pod: %w", scenario.Name, err)
	}

	// Delete PodNetworkInstance
	err = helpers.DeletePodNetworkInstance(kubeconfig, pnName, pniName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete PNI: %w", scenario.Name, err)
	}

	// Delete PodNetwork
	err = helpers.DeletePodNetwork(kubeconfig, pnName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete PN: %w", scenario.Name, err)
	}

	// Delete namespace
	err = helpers.DeleteNamespace(kubeconfig, pnName)
	if err != nil {
		return fmt.Errorf("scenario %s: failed to delete namespace: %w", scenario.Name, err)
	}

	fmt.Printf("Successfully deleted scenario: %s\n", scenario.Name)
	return nil
}

// CreateAllScenarios creates resources for all test scenarios
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

// DeleteAllScenarios deletes resources for all test scenarios
// Strategy: Delete all pods first, then delete shared PNI/PN/Namespace resources
func DeleteAllScenarios(testScenarios TestScenarios) error {
	// Phase 1: Delete all pods first
	fmt.Printf("\n=== Phase 1: Deleting all pods ===\n")
	for _, scenario := range testScenarios.Scenarios {
		kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", scenario.Cluster)
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
		kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", scenario.Cluster)
		vnetShort := strings.TrimPrefix(scenario.VnetName, "cx_vnet_")
		vnetShort = strings.ReplaceAll(vnetShort, "_", "-")
		subnetNameSafe := strings.ReplaceAll(scenario.SubnetName, "_", "-")
		pnName := fmt.Sprintf("pn-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)
		pniName := fmt.Sprintf("pni-%s-%s-%s", testScenarios.BuildID, vnetShort, subnetNameSafe)

		// Create unique key for this vnet/subnet/cluster combination
		resourceKey := fmt.Sprintf("%s:%s", scenario.Cluster, pnName)

		// Skip if we already deleted resources for this combination
		if resourceGroups[resourceKey] {
			continue
		}
		resourceGroups[resourceKey] = true

		fmt.Printf("\nDeleting shared resources for %s/%s on %s\n", scenario.VnetName, scenario.SubnetName, scenario.Cluster)

		// Delete PodNetworkInstance
		err := helpers.DeletePodNetworkInstance(kubeconfig, pnName, pniName)
		if err != nil {
			fmt.Printf("Warning: Failed to delete PNI %s: %v\n", pniName, err)
		}

		// Delete PodNetwork
		err = helpers.DeletePodNetwork(kubeconfig, pnName)
		if err != nil {
			fmt.Printf("Warning: Failed to delete PN %s: %v\n", pnName, err)
		}

		// Delete namespace
		err = helpers.DeleteNamespace(kubeconfig, pnName)
		if err != nil {
			fmt.Printf("Warning: Failed to delete namespace %s: %v\n", pnName, err)
		}
	}

	// Phase 3: Verify no MTPNC resources are stuck
	fmt.Printf("\n=== Phase 3: Verifying MTPNC cleanup ===\n")
	clustersChecked := make(map[string]bool)

	for _, scenario := range testScenarios.Scenarios {
		// Check each cluster only once
		if clustersChecked[scenario.Cluster] {
			continue
		}
		clustersChecked[scenario.Cluster] = true

		kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", scenario.Cluster)
		fmt.Printf("Checking for pending MTPNC resources in cluster %s\n", scenario.Cluster)

		err := helpers.VerifyNoMTPNC(kubeconfig, testScenarios.BuildID)
		if err != nil {
			fmt.Printf("WARNING: Found pending MTPNC resources in cluster %s: %v\n", scenario.Cluster, err)
			// Don't fail the test, just warn - MTPNC deletion might be in progress
		} else {
			fmt.Printf("✓ No pending MTPNC resources found in cluster %s\n", scenario.Cluster)
		}
	}

	fmt.Printf("\n=== All scenarios deleted ===\n")
	return nil
}

// DeleteTestResources deletes all test resources in reverse order
func DeleteTestResources(kubeconfig, pnName, pniName string) error {
	// Delete pods (first two nodes only, matching creation)
	for i := 0; i < 2; i++ {
		podName := fmt.Sprintf("pod-c2-%d", i)
		err := helpers.DeletePod(kubeconfig, pnName, podName)
		if err != nil {
			return fmt.Errorf("failed to delete pod %s: %w", podName, err)
		}
	}

	// Delete PodNetworkInstance
	err := helpers.DeletePodNetworkInstance(kubeconfig, pnName, pniName)
	if err != nil {
		return fmt.Errorf("failed to delete PodNetworkInstance: %w", err)
	}

	// Delete PodNetwork
	err = helpers.DeletePodNetwork(kubeconfig, pnName)
	if err != nil {
		return fmt.Errorf("failed to delete PodNetwork: %w", err)
	}

	// Delete namespace
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

// RunConnectivityTest tests HTTP connectivity between two pods
func RunConnectivityTest(test ConnectivityTest) error {
	// Get kubeconfig for the source cluster
	sourceKubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", test.Cluster)

	// Get kubeconfig for the destination cluster (default to source cluster if not specified)
	destKubeconfig := sourceKubeconfig
	if test.DestCluster != "" {
		destKubeconfig = fmt.Sprintf("/tmp/%s.kubeconfig", test.DestCluster)
	}

	// Get destination pod's eth1 IP (delegated subnet IP for cross-VNet connectivity)
	// This is the IP that is subject to NSG rules, not the overlay eth0 IP
	destIP, err := helpers.GetPodDelegatedIP(destKubeconfig, test.DestNamespace, test.DestinationPod)
	if err != nil {
		return fmt.Errorf("failed to get destination pod delegated IP: %w", err)
	}

	fmt.Printf("Testing connectivity from %s/%s (cluster: %s) to %s/%s (cluster: %s, eth1: %s) on port 8080\n",
		test.SourceNamespace, test.SourcePod, test.Cluster,
		test.DestNamespace, test.DestinationPod, test.DestCluster, destIP)

	// Run curl command from source pod to destination pod using eth1 IP
	// Using -m 3 for 3 second timeout (short because netcat closes connection immediately)
	// Using --interface eth1 to force traffic through delegated subnet interface
	// Using --http0.9 to allow HTTP/0.9 responses from netcat (which sends raw text without proper HTTP headers)
	// Exit code 28 (timeout) is OK if we received data, since netcat doesn't properly close the connection
	curlCmd := fmt.Sprintf("curl --http0.9 --interface eth1 -m 3 http://%s:8080/", destIP)

	output, err := helpers.ExecInPod(sourceKubeconfig, test.SourceNamespace, test.SourcePod, curlCmd)
	// Check if we received data even if curl timed out (exit code 28)
	// Netcat closes the connection without proper HTTP close, causing curl to timeout
	// But if we got the expected response, the connectivity test is successful
	if err != nil {
		if strings.Contains(err.Error(), "exit status 28") && strings.Contains(output, "TCP Connection Success") {
			// Timeout but we got the data - this is OK with netcat
			fmt.Printf("Connectivity successful (timeout OK, data received)! Response preview: %s\n", truncateString(output, 100))
			return nil
		}
		return fmt.Errorf("connectivity test failed: %w\nOutput: %s", err, output)
	}

	fmt.Printf("Connectivity successful! Response preview: %s\n", truncateString(output, 100))
	return nil
}

// Helper function to truncate long strings
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GenerateStorageSASToken generates a SAS token for a blob in a storage account
func GenerateStorageSASToken(storageAccountName, containerName, blobName string) (string, error) {
	// Calculate expiry time: 7 days from now (Azure CLI limit)
	expiryTime := time.Now().UTC().Add(7 * 24 * time.Hour).Format("2006-01-02")

	// Try account key first (more reliable, no RBAC delay)
	cmd := exec.Command("az", "storage", "blob", "generate-sas",
		"--account-name", storageAccountName,
		"--container-name", containerName,
		"--name", blobName,
		"--permissions", "r",
		"--expiry", expiryTime,
		"--output", "tsv")

	out, err := cmd.CombinedOutput()
	sasToken := strings.TrimSpace(string(out))

	// Check if account key method produced valid token
	accountKeyWorked := err == nil && !strings.Contains(sasToken, "WARNING") &&
		!strings.Contains(sasToken, "ERROR") && (strings.Contains(sasToken, "sv=") || strings.Contains(sasToken, "sig="))

	if !accountKeyWorked {
		// If account key fails, fall back to user delegation (requires RBAC)
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

	// Remove any surrounding quotes that might be added by some shells
	sasToken = strings.Trim(sasToken, "\"'")

	// Validate SAS token format - should start with typical SAS parameters
	if !strings.Contains(sasToken, "sv=") && !strings.Contains(sasToken, "sig=") {
		return "", fmt.Errorf("%w (missing sv= or sig=): %s", ErrSASTokenInvalid, sasToken)
	}

	return sasToken, nil
}

// GetStoragePrivateEndpoint retrieves the private IP address of a storage account's private endpoint
func GetStoragePrivateEndpoint(storageAccountName string) (string, error) {
	// Return the storage account blob endpoint FQDN
	// This will resolve to the private IP via Private DNS Zone
	return storageAccountName + ".blob.core.windows.net", nil
}

// RunPrivateEndpointTest tests connectivity from a pod to a private endpoint (storage account)
func RunPrivateEndpointTest(test ConnectivityTest) error {
	// Get kubeconfig for the cluster
	kubeconfig := fmt.Sprintf("/tmp/%s.kubeconfig", test.SourceCluster)

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
		// Check for HTTP errors in wget output
		if strings.Contains(output, "ERROR 403") || strings.Contains(output, "ERROR 401") {
			return fmt.Errorf("%w\nOutput: %s", ErrHTTPAuthError, truncateString(output, 500))
		}
		if strings.Contains(output, "ERROR 404") {
			return fmt.Errorf("%w\nOutput: %s", ErrBlobNotFound, truncateString(output, 500))
		}
		return fmt.Errorf("private endpoint connectivity test failed: %w\nOutput: %s", err, truncateString(output, 500))
	}

	// Verify we got valid content
	if strings.Contains(output, "Hello") || strings.Contains(output, "200 OK") || strings.Contains(output, "saved") {
		fmt.Printf("Private endpoint access successful!\n")
		return nil
	}

	return fmt.Errorf("%w\nOutput: %s", ErrUnexpectedBlobResponse, truncateString(output, 500))
}

// ExecInPodWithTimeout executes a command in a pod with a custom timeout
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
