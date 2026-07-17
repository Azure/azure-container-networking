package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	envFaultScenario       = "MIGRATION_FAULT_SCENARIO"
	envFaultOS             = "MIGRATION_FAULT_OS"
	envFaultCNI            = "MIGRATION_FAULT_CNI"
	envFaultRunID          = "MIGRATION_FAULT_RUN_ID"
	envFaultArtifactDir    = "MIGRATION_FAULT_ARTIFACT_DIR"
	envFaultScaleReplicas  = "MIGRATION_FAULT_SCALE_REPLICAS"
	envFaultTimeoutMinutes = "MIGRATION_FAULT_TIMEOUT_MINUTES"
	envFaultWorkloadImage  = "MIGRATION_FAULT_WORKLOAD_IMAGE"
	envValidateBackend     = "VALIDATE_STATE_BACKEND"

	faultTokenEnv    = "CNS_TEST_FAULT_INJECTION_TOKEN"
	faultTokenHeader = "X-CNS-Test-Fault-Token"
	faultAPIPath     = "/debug/faultinjection"

	defaultScaleReplicas = 20
	defaultTimeout       = 45 * time.Minute
	defaultWorkloadImage = "mcr.microsoft.com/oss/kubernetes/pause:3.6"
)

type scenario string

const (
	scenarioAll                     scenario = "all"
	scenarioAddBeforeEndpointCommit scenario = "add-before-endpoint-commit"
	scenarioDeleteAfterIntentCommit scenario = "delete-after-intent-commit"
	scenarioEndpointPatch           scenario = "endpoint-patch"
	scenarioRestartDuringScale      scenario = "restart-during-scale"
	faultPointAddBeforeEndpoint     string   = "add-before-endpoint-commit"
	faultPointDeleteAfterIntent     string   = "delete-after-intent-commit"
	faultPointPatchBeforeEndpoint   string   = "patch-before-endpoint-commit"
	linuxCNSDaemonSet                        = "azure-cns"
	windowsCNSDaemonSet                      = "azure-cns-win"
	linuxCNSLabelSelector                    = "k8s-app=azure-cns"
	windowsCNSLabelSelector                  = "k8s-app=azure-cns-win"
)

type faultConfig struct {
	Scenario      scenario
	OS            string
	CNI           string
	RunID         string
	ArtifactDir   string
	ScaleReplicas int32
	Timeout       time.Duration
	WorkloadImage string
}

type envBackup struct {
	Present bool
	Value   corev1.EnvVar
}

type migrationCNSConfig struct {
	StateStoreBackend   string `json:"StateStoreBackend"`
	ManageEndpointState bool   `json:"ManageEndpointState"`
}

func loadFaultConfig(getenv func(string) string) (faultConfig, error) {
	cfg := faultConfig{
		Scenario:      scenario(valueOrDefault(getenv(envFaultScenario), string(scenarioAll))),
		OS:            strings.ToLower(valueOrDefault(getenv(envFaultOS), "linux")),
		CNI:           strings.ToLower(valueOrDefault(getenv(envFaultCNI), "cilium")),
		RunID:         getenv(envFaultRunID),
		ArtifactDir:   getenv(envFaultArtifactDir),
		ScaleReplicas: defaultScaleReplicas,
		Timeout:       defaultTimeout,
		WorkloadImage: valueOrDefault(getenv(envFaultWorkloadImage), defaultWorkloadImage),
	}
	if _, err := cfg.scenarios(); err != nil {
		return faultConfig{}, err
	}
	if cfg.OS != "linux" && cfg.OS != "windows" {
		return faultConfig{}, fmt.Errorf("unsupported migration fault OS %q", cfg.OS)
	}
	if !supportedCNI(cfg.OS, cfg.CNI) {
		return faultConfig{}, fmt.Errorf("unsupported migration fault CNI %q for OS %q", cfg.CNI, cfg.OS)
	}
	if cfg.RunID == "" {
		return faultConfig{}, fmt.Errorf("%s is required", envFaultRunID)
	}
	if cfg.ArtifactDir == "" {
		return faultConfig{}, fmt.Errorf("%s is required", envFaultArtifactDir)
	}
	if strings.ToLower(getenv(envValidateBackend)) != "bolt" {
		return faultConfig{}, fmt.Errorf("%s must be bolt", envValidateBackend)
	}

	if raw := getenv(envFaultScaleReplicas); raw != "" {
		replicas, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || replicas < 2 {
			return faultConfig{}, fmt.Errorf("%s must be an integer greater than one", envFaultScaleReplicas)
		}
		cfg.ScaleReplicas = int32(replicas)
	}
	if raw := getenv(envFaultTimeoutMinutes); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes <= 0 {
			return faultConfig{}, fmt.Errorf("%s must be a positive integer", envFaultTimeoutMinutes)
		}
		cfg.Timeout = time.Duration(minutes) * time.Minute
	}
	return cfg, nil
}

func supportedCNI(osName, cni string) bool {
	switch osName {
	case "linux":
		return cni == "cilium" || cni == "cniv1" || cni == "cniv2" || cni == "dualstack"
	case "windows":
		return cni == "cniv1" || cni == "cniv2" || cni == "stateless"
	default:
		return false
	}
}

func (cfg faultConfig) scenarios() ([]scenario, error) {
	switch cfg.Scenario {
	case scenarioAll:
		return []scenario{
			scenarioAddBeforeEndpointCommit,
			scenarioDeleteAfterIntentCommit,
			scenarioEndpointPatch,
			scenarioRestartDuringScale,
		}, nil
	case scenarioAddBeforeEndpointCommit,
		scenarioDeleteAfterIntentCommit,
		scenarioEndpointPatch,
		scenarioRestartDuringScale:
		return []scenario{cfg.Scenario}, nil
	default:
		return nil, fmt.Errorf("unsupported migration fault scenario %q", cfg.Scenario)
	}
}

func faultPointForScenario(value scenario) string {
	switch value {
	case scenarioDeleteAfterIntentCommit:
		return faultPointDeleteAfterIntent
	case scenarioEndpointPatch:
		return faultPointPatchBeforeEndpoint
	default:
		return faultPointAddBeforeEndpoint
	}
}

func cnsDaemonSetForOS(osName string) (string, string) {
	if osName == "windows" {
		return windowsCNSDaemonSet, windowsCNSLabelSelector
	}
	return linuxCNSDaemonSet, linuxCNSLabelSelector
}

func sanitizeResourceName(value string, maxLength int) string {
	var builder strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastHyphen = false
		case !lastHyphen:
			builder.WriteByte('-')
			lastHyphen = true
		}
	}
	name := strings.Trim(builder.String(), "-")
	if name == "" {
		name = "run"
	}
	if len(name) <= maxLength {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:4])
	return strings.Trim(name[:maxLength-len(suffix)-1], "-") + "-" + suffix
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func validateMigrationCNSConfig(raw []byte) error {
	var config migrationCNSConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return fmt.Errorf("decoding CNS config: %w", err)
	}
	if strings.ToLower(config.StateStoreBackend) != "bolt" {
		return fmt.Errorf("state store backend must be bolt, got %q", config.StateStoreBackend)
	}
	if !config.ManageEndpointState {
		return fmt.Errorf("managed endpoint state must be enabled")
	}
	return nil
}

func findCNSContainer(containers []corev1.Container) (int, error) {
	for i := range containers {
		for _, port := range containers[i].Ports {
			if port.ContainerPort == 10090 {
				return i, nil
			}
		}
	}
	for i := range containers {
		for _, env := range containers[i].Env {
			if env.Name == "CNS_CONFIGURATION_PATH" {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("container exposing the CNS API was not found")
}

func setContainerEnv(container *corev1.Container, name, value string) envBackup {
	for i := range container.Env {
		if container.Env[i].Name == name {
			backup := envBackup{Present: true, Value: container.Env[i]}
			container.Env[i] = corev1.EnvVar{Name: name, Value: value}
			return backup
		}
	}
	container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
	return envBackup{}
}

func restoreContainerEnv(container *corev1.Container, name, currentValue string, backup envBackup) bool {
	for i := range container.Env {
		if container.Env[i].Name != name || container.Env[i].Value != currentValue {
			continue
		}
		if backup.Present {
			container.Env[i] = backup.Value
		} else {
			container.Env = append(container.Env[:i], container.Env[i+1:]...)
		}
		return true
	}
	return false
}

func selectCNSTarget(pods []corev1.Pod, nodeName string) (corev1.Pod, error) {
	var selected *corev1.Pod
	for i := range pods {
		pod := &pods[i]
		if nodeName != "" && pod.Spec.NodeName != nodeName {
			continue
		}
		if pod.DeletionTimestamp != nil || !isPodReady(*pod) {
			continue
		}
		if selected == nil || pod.Spec.NodeName < selected.Spec.NodeName ||
			(pod.Spec.NodeName == selected.Spec.NodeName && pod.Name < selected.Name) {
			selected = pod
		}
	}
	if selected == nil {
		return corev1.Pod{}, fmt.Errorf("no ready CNS pod found")
	}
	return *selected.DeepCopy(), nil
}

func isPodReady(pod corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func exactPodDeleteOptions(uid types.UID) metav1.DeleteOptions {
	gracePeriod := int64(0)
	return metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
		Preconditions: &metav1.Preconditions{
			UID: &uid,
		},
	}
}

func workloadPodDeleteOptions(uid types.UID) metav1.DeleteOptions {
	return metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			UID: &uid,
		},
	}
}
