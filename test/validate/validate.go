package validate

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var privilegedDaemonSetPathMap = map[string]string{
	"windows": "../manifests/load/privileged-daemonset-windows.yaml",
	"linux":   "../manifests/load/privileged-daemonset.yaml",
}

var nodeSelectorMap = map[string]string{
	"windows": "kubernetes.io/os=windows",
	"linux":   "kubernetes.io/os=linux",
}

// IPv4 overlay Linux and windows nodes must have this label
var v4OverlayNodeLabels = map[string]string{
	"kubernetes.azure.com/podnetwork-type": "overlay",
}

// dualstack overlay Linux and windows nodes must have these labels
var dualstackOverlayNodeLabels = map[string]string{
	"kubernetes.azure.com/podnetwork-type":   "overlay",
	"kubernetes.azure.com/podv6network-type": "overlay",
}

const (
	privilegedLabelSelector  = "app=privileged-daemonset"
	privilegedNamespace      = "kube-system"
	IPv4ExpectedIPCount      = 1
	DualstackExpectedIPCount = 2
)

type Validator struct {
	clientset   *kubernetes.Clientset
	config      *rest.Config
	checks      []check
	namespace   string
	cni         string
	restartCase bool
	os          string
	summary     ValidationSummary
}

type ValidationSummary struct {
	GeneratedAt string                 `json:"generatedAt"`
	OS          string                 `json:"os"`
	CNI         string                 `json:"cni"`
	Namespace   string                 `json:"namespace"`
	RestartCase bool                   `json:"restartCase"`
	Checks      []ValidationCheckEntry `json:"checks,omitempty"`
}

type ValidationCheckEntry struct {
	CheckName      string   `json:"checkName"`
	NodeName       string   `json:"nodeName"`
	ExpectedCount  int      `json:"expectedCount"`
	ActualCount    int      `json:"actualCount"`
	Attempts       int      `json:"attempts"`
	DurationMS     int64    `json:"durationMS"`
	Converged      bool     `json:"converged"`
	MissingIPs     []string `json:"missingIPs,omitempty"`
	UnexpectedIPs  []string `json:"unexpectedIPs,omitempty"`
	DuplicateIPs   []string `json:"duplicateIPs,omitempty"`
	ValidationPass bool     `json:"validationPass"`
}

type check struct {
	name             string
	stateFileIPs     func([]byte) (map[string]string, error)
	podLabelSelector string
	podNamespace     string
	containerName    string
	cmd              []string
}

func CreateValidator(ctx context.Context, clientset *kubernetes.Clientset, config *rest.Config, namespace, cni string, restartCase bool, os string) (*Validator, error) {
	// deploy privileged pod
	privilegedDaemonSet := acnk8s.MustParseDaemonSet(privilegedDaemonSetPathMap[os])
	daemonsetClient := clientset.AppsV1().DaemonSets(privilegedNamespace)
	acnk8s.MustCreateDaemonset(ctx, daemonsetClient, privilegedDaemonSet)

	// Ensures that pods have been replaced if test is re-run after failure
	if err := acnk8s.WaitForPodDaemonset(ctx, clientset, privilegedNamespace, privilegedDaemonSet.Name, privilegedLabelSelector); err != nil {
		return nil, errors.Wrap(err, "unable to wait for daemonset")
	}

	var checks []check
	switch os {
	case "windows":
		checks = windowsChecksMap[cni]
		err := acnk8s.RestartKubeProxyService(ctx, clientset, privilegedNamespace, privilegedLabelSelector, config)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to restart kubeproxy")
		}
	case "linux":
		checks = linuxChecksMap[cni]
	default:
		return nil, errors.Errorf("unsupported os: %s", os)
	}

	return &Validator{
		clientset:   clientset,
		config:      config,
		namespace:   namespace,
		cni:         cni,
		restartCase: restartCase,
		checks:      checks,
		os:          os,
		summary: ValidationSummary{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			OS:          os,
			CNI:         cni,
			Namespace:   namespace,
			RestartCase: restartCase,
		},
	}, nil
}

func (v *Validator) Validate(ctx context.Context) error {
	defer v.writeSummaryIfRequested()

	log.Printf("Validating State File")
	err := v.ValidateStateFile(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to validate state file")
	}

	if v.os == "linux" {
		// We are restarting the systmemd network and checking that the connectivity works after the restart. For more details: https://github.com/cilium/cilium/issues/18706
		log.Printf("Validating the restart network scenario")
		err = v.validateRestartNetwork(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to validate restart network scenario")
		}
	}
	return nil
}

func (v *Validator) ValidateStateFile(ctx context.Context) error {
	for _, check := range v.checks {
		err := v.validateIPs(ctx, check.stateFileIPs, check.cmd, check.name, check.podNamespace, check.podLabelSelector, check.containerName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Validator) validateIPs(ctx context.Context, stateFileIps stateFileIpsFunc, cmd []string, checkType, namespace, labelSelector, containerName string) error {
	log.Printf("Validating %s state file for %s on %s", checkType, v.cni, v.os)
	nodes, err := acnk8s.GetNodeListByLabelSelector(ctx, v.clientset, nodeSelectorMap[v.os])
	if err != nil {
		return errors.Wrapf(err, "failed to get node list")
	}

	maxAttempts := envInt("VALIDATE_CONVERGENCE_ATTEMPTS", 1)
	intervalSeconds := envInt("VALIDATE_CONVERGENCE_INTERVAL_SECONDS", 0)

	for index := range nodes.Items {
		nodeName := nodes.Items[index].Name
		started := time.Now()
		attempts := 0
		converged := false
		var comparison ipComparisonResult

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			attempts = attempt

			pod, err := acnk8s.GetPodsByNode(ctx, v.clientset, namespace, labelSelector, nodeName)
			if err != nil {
				return errors.Wrapf(err, "failed to get privileged pod")
			}
			if len(pod.Items) == 0 {
				return errors.Errorf("there are no privileged pods on node - %v", nodeName)
			}
			podName := pod.Items[0].Name

			log.Printf("Executing command %s on pod %s, container %s", cmd, podName, containerName)
			result, _, err := acnk8s.ExecCmdOnPod(ctx, v.clientset, namespace, podName, containerName, cmd, v.config, true)
			if err != nil {
				return errors.Wrapf(err, "failed to exec into privileged pod - %s", podName)
			}

			filePodIps, err := stateFileIps(result)
			if err != nil {
				return errors.Wrapf(err, "failed to get pod ips from state file on node %v", nodeName)
			}
			if len(filePodIps) == 0 && v.restartCase {
				comparison = ipComparisonResult{}
				converged = true
				log.Printf("No pods found on node %s", nodeName)
				break
			}

			podIps := getPodIPsWithoutNodeIP(ctx, v.clientset, nodes.Items[index])
			comparison = compareIPsDetailed(filePodIps, podIps)
			if !comparison.HasMismatch() {
				converged = true
				break
			}

			if attempt < maxAttempts && intervalSeconds > 0 {
				time.Sleep(time.Duration(intervalSeconds) * time.Second)
			}
		}

		v.summary.Checks = append(v.summary.Checks, ValidationCheckEntry{
			CheckName:      checkType,
			NodeName:       nodeName,
			ExpectedCount:  comparison.ExpectedCount,
			ActualCount:    comparison.ActualCount,
			Attempts:       attempts,
			DurationMS:     time.Since(started).Milliseconds(),
			Converged:      converged,
			MissingIPs:     comparison.MissingIPs,
			UnexpectedIPs:  comparison.UnexpectedIPs,
			DuplicateIPs:   comparison.DuplicateIPs,
			ValidationPass: converged && !comparison.HasMismatch(),
		})

		if !converged || comparison.HasMismatch() {
			return errors.Errorf(
				"State file validation failed for %s on node %s after %d/%d attempts: expected=%d actual=%d missing=%v unexpected=%v duplicate=%v",
				checkType,
				nodeName,
				attempts,
				maxAttempts,
				comparison.ExpectedCount,
				comparison.ActualCount,
				comparison.MissingIPs,
				comparison.UnexpectedIPs,
				comparison.DuplicateIPs,
			)
		}
	}
	log.Printf("State file validation for %s passed", checkType)
	return nil
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("invalid %s=%q, using fallback %d", name, raw, fallback)
		return fallback
	}
	if v < 0 {
		log.Printf("invalid %s=%q (<0), using fallback %d", name, raw, fallback)
		return fallback
	}

	return v
}

func (v *Validator) writeSummaryIfRequested() {
	summaryPath := os.Getenv("VALIDATE_SUMMARY_PATH")
	if summaryPath == "" {
		return
	}

	v.summary.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(v.summary, "", "  ")
	if err != nil {
		log.Printf("failed to marshal validation summary: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		log.Printf("failed to create summary directory: %v", err)
		return
	}

	if err := os.WriteFile(summaryPath, raw, 0o644); err != nil {
		log.Printf("failed to write validation summary %s: %v", summaryPath, err)
		return
	}

	log.Printf("validation summary written to %s", summaryPath)
}

func validateNodeProperties(nodes *corev1.NodeList, labels map[string]string, expectedIPCount int) error {
	log.Print("Validating Node properties")

	for index := range nodes.Items {
		nodeName := nodes.Items[index].ObjectMeta.Name
		// check nodes status;
		// nodes status should be ready after cluster is created
		nodeConditions := nodes.Items[index].Status.Conditions
		if nodeConditions[len(nodeConditions)-1].Type != corev1.NodeReady {
			return errors.Errorf("node %s status is not ready", nodeName)
		}

		// get node labels
		nodeLabels := nodes.Items[index].ObjectMeta.GetLabels()
		for key := range nodeLabels {
			if label, ok := labels[key]; ok {
				log.Printf("label %s is correctly shown on the node %+v", key, nodeName)
				if label != overlayClusterLabelName {
					return errors.Errorf("node %s overlay label name is wrong; expected label:%s but actual label:%s", nodeName, overlayClusterLabelName, label)
				}
			}
		}

		// check if node has correct number of internal IPs
		internalIPCount := 0
		for _, address := range nodes.Items[index].Status.Addresses {
			if address.Type == "InternalIP" {
				internalIPCount++
			}
		}
		if internalIPCount != expectedIPCount {
			return errors.Errorf("number of node internal IPs: %d does not match expected number of IPs %d", internalIPCount, expectedIPCount)
		}
	}
	return nil
}

func (v *Validator) ValidateV4OverlayControlPlane(ctx context.Context) error {
	nodes, err := acnk8s.GetNodeListByLabelSelector(ctx, v.clientset, nodeSelectorMap[v.os])
	if err != nil {
		return errors.Wrap(err, "failed to get node list")
	}

	if err := validateNodeProperties(nodes, v4OverlayNodeLabels, IPv4ExpectedIPCount); err != nil {
		return errors.Wrap(err, "failed to validate IPv4 overlay node properties")
	}

	if v.os == "windows" {
		if err := validateHNSNetworkState(ctx, nodes, v.clientset, v.config); err != nil {
			return errors.Wrap(err, "failed to validate IPv4 overlay HNS network state")
		}
	}

	return nil
}

func (v *Validator) ValidateDualStackControlPlane(ctx context.Context) error {
	nodes, err := acnk8s.GetNodeListByLabelSelector(ctx, v.clientset, nodeSelectorMap[v.os])
	if err != nil {
		return errors.Wrap(err, "failed to get node list")
	}

	if err := validateNodeProperties(nodes, dualstackOverlayNodeLabels, DualstackExpectedIPCount); err != nil {
		return errors.Wrap(err, "failed to validate dualstack overlay node properties")
	}

	if v.os == "windows" {
		if err := validateHNSNetworkState(ctx, nodes, v.clientset, v.config); err != nil {
			return errors.Wrap(err, "failed to validate dualstack overlay HNS network state")
		}
	}

	return nil
}

func (v *Validator) Cleanup(ctx context.Context) {
	// deploy privileged pod
	privilegedDaemonSet := acnk8s.MustParseDaemonSet(privilegedDaemonSetPathMap[v.os])
	daemonsetClient := v.clientset.AppsV1().DaemonSets(privilegedNamespace)
	acnk8s.MustDeleteDaemonset(ctx, daemonsetClient, privilegedDaemonSet)
}

func cnsCacheStateFileIps(result []byte) (map[string]string, error) {
	var cnsLocalCache CNSLocalCache

	err := json.Unmarshal(result, &cnsLocalCache)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cns local cache")
	}

	cnsPodIps := make(map[string]string)
	for index := range cnsLocalCache.IPConfigurationStatus {
		cnsPodIps[cnsLocalCache.IPConfigurationStatus[index].IPAddress] = cnsLocalCache.IPConfigurationStatus[index].PodInfo.Name()
	}
	return cnsPodIps, nil
}

func cnsManagedStateFileIps(result []byte) (map[string]string, error) {
	var cnsResult CnsManagedState
	err := json.Unmarshal(result, &cnsResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal cns endpoint list")
	}

	cnsPodIps := make(map[string]string)
	for _, v := range cnsResult.Endpoints {
		for ifName, ip := range v.IfnameToIPMap {
			if ifName == "eth0" {
				cnsPodIps[ip.IPv4[0].IP.String()] = v.PodName
			}
		}
	}
	return cnsPodIps, nil
}
