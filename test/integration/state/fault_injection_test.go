//go:build load

package state

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	persistentstate "github.com/Azure/azure-container-networking/cns/state"
	integrationk8s "github.com/Azure/azure-container-networking/test/integration"
	acnk8s "github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/Azure/azure-container-networking/test/validate"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

const (
	kubeSystemNamespace = "kube-system"
	cnsPort             = 10090
	faultWaitTimeout    = 8 * time.Minute
	rolloutWaitTimeout  = 15 * time.Minute
)

type clusterHarness struct {
	clientset       *kubernetes.Clientset
	restConfig      *rest.Config
	cfg             faultConfig
	token           string
	namespace       string
	namespaceUID    types.UID
	daemonSetName   string
	labelSelector   string
	cnsContainer    string
	envBackup       envBackup
	validator       *validate.Validator
	validationReady bool
	faultEnabled    bool
}

type cnsTarget struct {
	PodName     string    `json:"podName"`
	PodUID      types.UID `json:"podUID"`
	NodeName    string    `json:"nodeName"`
	Container   string    `json:"container"`
	ContainerID string    `json:"containerID"`
	Restart     int32     `json:"restartCount"`
}

type faultControl struct {
	forwarder *integrationk8s.PortForwarder
	client    *http.Client
	baseURL   string
	token     string
}

type faultTarget struct {
	PodName      string `json:"podName,omitempty"`
	PodNamespace string `json:"podNamespace"`
}

type faultStatus struct {
	Point  string      `json:"point"`
	Target faultTarget `json:"target"`
	State  string      `json:"state"`
}

type workload struct {
	pod        *corev1.Pod
	deployment *appsv1.Deployment
}

func TestMigrationFaultInjection(t *testing.T) {
	cfg, err := loadFaultConfig(os.Getenv)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(cfg.ArtifactDir, 0o755))

	ctx, cancel := context.WithTimeout(t.Context(), cfg.Timeout)
	defer cancel()

	harness := newClusterHarness(t, ctx, cfg)

	scenarios, err := cfg.scenarios()
	require.NoError(t, err)
	for _, value := range scenarios {
		if !t.Run(string(value), func(t *testing.T) {
			harness.runScenario(t, ctx, value)
		}) {
			return
		}
	}
}

func newClusterHarness(t *testing.T, ctx context.Context, cfg faultConfig) *clusterHarness {
	t.Helper()
	token, err := randomToken()
	require.NoError(t, err)
	daemonSetName, labelSelector := cnsDaemonSetForOS(cfg.OS)
	harness := &clusterHarness{
		clientset:     acnk8s.MustGetClientset(),
		restConfig:    acnk8s.MustGetRestConfig(),
		cfg:           cfg,
		token:         token,
		namespace:     sanitizeResourceName("cns-migration-fi-"+cfg.RunID, 63),
		daemonSetName: daemonSetName,
		labelSelector: labelSelector,
	}
	t.Cleanup(func() {
		if err := harness.cleanup(); err != nil {
			t.Errorf("migration fault cleanup failed: %v", err)
		}
	})

	require.NoError(t, harness.validateCNSConfig(ctx))
	require.NoError(t, harness.enableFaultInjection(ctx))
	require.NoError(t, harness.createNamespace(ctx))

	validator, err := validate.CreateValidator(ctx, harness.clientset, harness.restConfig, harness.namespace, cfg.CNI, true, cfg.OS)
	require.NoError(t, err)
	harness.validator = validator
	harness.validationReady = true

	target, err := harness.selectTarget(ctx, "")
	require.NoError(t, err)
	control, err := newFaultControl(ctx, harness.restConfig, target, token)
	require.NoError(t, err)
	defer control.close()
	status, err := control.status(ctx)
	require.NoError(t, err)
	require.Equal(t, "idle", status.State)
	raw, err := control.debug(ctx, "/debug/persistentstate", []byte("{}"))
	require.NoError(t, err)
	require.NoError(t, validatePersistentState(raw))
	return harness
}

func (harness *clusterHarness) validateCNSConfig(ctx context.Context) error {
	configMapName := cnsConfigMapForOS(harness.cfg.OS)
	configMap, err := harness.clientset.CoreV1().ConfigMaps(kubeSystemNamespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting CNS configmap %q: %w", configMapName, err)
	}
	raw := []byte(configMap.Data["cns_config.json"])
	if err := validateMigrationCNSConfig(raw); err != nil {
		return err
	}
	return writeFile(filepath.Join(harness.cfg.ArtifactDir, "cns-config.json"), raw)
}

func (harness *clusterHarness) cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), rolloutWaitTimeout)
	defer cancel()
	var cleanupErrors []error
	if harness.faultEnabled {
		if err := harness.disableFaultInjection(ctx); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if harness.namespaceUID != "" {
		if err := harness.clientset.CoreV1().Namespaces().Delete(
			ctx,
			harness.namespace,
			metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &harness.namespaceUID}},
		); err != nil && !apierrors.IsNotFound(err) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if harness.validationReady {
		harness.validator.Cleanup(ctx)
	}
	return errors.Join(cleanupErrors...)
}

func (harness *clusterHarness) runScenario(t *testing.T, ctx context.Context, value scenario) {
	t.Helper()
	scenarioDir := filepath.Join(harness.cfg.ArtifactDir, sanitizeResourceName(string(value), 63))
	require.NoError(t, os.MkdirAll(scenarioDir, 0o755))

	target, err := harness.selectTarget(ctx, "")
	require.NoError(t, err)
	currentTarget := target
	var activeControl *faultControl
	defer func() {
		if t.Failed() {
			harness.captureFailure(context.Background(), scenarioDir, currentTarget)
		}
		if activeControl != nil {
			activeControl.close()
		}
	}()

	work := workload{}
	switch value {
	case scenarioDeleteAfterIntentCommit:
		work.pod, err = harness.createPod(ctx, value, target.NodeName)
		require.NoError(t, err)
		require.NoError(t, harness.waitForPodReady(ctx, work.pod.Name))
		target, err = harness.selectTarget(ctx, target.NodeName)
		require.NoError(t, err)
		currentTarget = target
	case scenarioRestartDuringScale:
		work.deployment, err = harness.createDeployment(ctx, value, target.NodeName)
		require.NoError(t, err)
	}

	activeControl, err = newFaultControl(ctx, harness.restConfig, target, harness.token)
	require.NoError(t, err)
	targetFilter := faultTarget{PodNamespace: harness.namespace}
	if value != scenarioRestartDuringScale {
		targetFilter.PodName = harness.workloadName(value)
	}
	require.NoError(t, activeControl.arm(ctx, faultPointForScenario(value), targetFilter))

	switch value {
	case scenarioAddBeforeEndpointCommit, scenarioEndpointPatch:
		work.pod, err = harness.createPod(ctx, value, target.NodeName)
	case scenarioDeleteAfterIntentCommit:
		err = harness.deletePodExact(ctx, *work.pod)
	case scenarioRestartDuringScale:
		err = harness.scaleDeployment(ctx, work.deployment.Name, harness.cfg.ScaleReplicas)
	default:
		err = fmt.Errorf("unsupported migration fault scenario %q", value)
	}
	require.NoError(t, err)
	require.NoError(t, activeControl.waitReached(ctx))
	require.NoError(t, harness.captureArtifacts(ctx, scenarioDir, "pre-kill", target, activeControl))
	status, err := activeControl.status(ctx)
	require.NoError(t, err)
	require.Equal(t, faultPointForScenario(value), status.Point)
	require.Equal(t, "reached", status.State)
	require.Equal(t, targetFilter, status.Target)

	require.NoError(t, harness.killTarget(ctx, target))
	activeControl.close()
	activeControl = nil

	replacement, err := harness.waitForReplacement(ctx, target)
	require.NoError(t, err)
	currentTarget = replacement
	require.NoError(t, harness.waitForWorkload(ctx, value, work))

	activeControl, err = newFaultControl(ctx, harness.restConfig, replacement, harness.token)
	require.NoError(t, err)
	require.NoError(t, harness.captureArtifacts(ctx, scenarioDir, "post-restart", replacement, activeControl))

	t.Setenv("VALIDATE_SUMMARY_PATH", filepath.Join(scenarioDir, "validation-summary.json"))
	validationErr := harness.validator.ValidateStateFile(ctx)
	require.NoError(t, writeFile(filepath.Join(scenarioDir, "validation.txt"), []byte(validationResult(validationErr))))
	require.NoError(t, validationErr)
	require.NoError(t, harness.cleanupWorkload(ctx, work))
}

func (harness *clusterHarness) enableFaultInjection(ctx context.Context) error {
	var generation int64
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		daemonSet, err := harness.clientset.AppsV1().DaemonSets(kubeSystemNamespace).Get(ctx, harness.daemonSetName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		index, err := findCNSContainer(daemonSet.Spec.Template.Spec.Containers)
		if err != nil {
			return err
		}
		container := &daemonSet.Spec.Template.Spec.Containers[index]
		for _, env := range container.Env {
			if env.Name == faultTokenEnv && env.Value != "" {
				return fmt.Errorf("%s is already configured on daemonset %s", faultTokenEnv, harness.daemonSetName)
			}
		}
		harness.cnsContainer = container.Name
		harness.envBackup = setContainerEnv(container, faultTokenEnv, harness.token)
		updated, err := harness.clientset.AppsV1().DaemonSets(kubeSystemNamespace).Update(ctx, daemonSet, metav1.UpdateOptions{})
		if err == nil {
			generation = updated.Generation
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("enabling CNS fault injection: %w", err)
	}
	harness.faultEnabled = true
	return harness.waitForDaemonSet(ctx, generation)
}

func (harness *clusterHarness) disableFaultInjection(ctx context.Context) error {
	var generation int64
	changed := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		changed = false
		daemonSet, err := harness.clientset.AppsV1().DaemonSets(kubeSystemNamespace).Get(ctx, harness.daemonSetName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		for i := range daemonSet.Spec.Template.Spec.Containers {
			if daemonSet.Spec.Template.Spec.Containers[i].Name != harness.cnsContainer {
				continue
			}
			changed = restoreContainerEnv(
				&daemonSet.Spec.Template.Spec.Containers[i],
				faultTokenEnv,
				harness.token,
				harness.envBackup,
			)
			break
		}
		if !changed {
			return nil
		}
		updated, err := harness.clientset.AppsV1().DaemonSets(kubeSystemNamespace).Update(ctx, daemonSet, metav1.UpdateOptions{})
		if err == nil {
			generation = updated.Generation
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("disabling CNS fault injection: %w", err)
	}
	if !changed {
		harness.faultEnabled = false
		return nil
	}
	if err := harness.waitForDaemonSet(ctx, generation); err != nil {
		return err
	}
	harness.faultEnabled = false
	return nil
}

func (harness *clusterHarness) waitForDaemonSet(ctx context.Context, generation int64) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, rolloutWaitTimeout, true, func(ctx context.Context) (bool, error) {
		daemonSet, err := harness.clientset.AppsV1().DaemonSets(kubeSystemNamespace).Get(ctx, harness.daemonSetName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		status := daemonSet.Status
		return status.DesiredNumberScheduled > 0 &&
			status.ObservedGeneration >= generation &&
			status.UpdatedNumberScheduled == status.DesiredNumberScheduled &&
			status.NumberReady == status.DesiredNumberScheduled &&
			status.NumberUnavailable == 0, nil
	})
}

func (harness *clusterHarness) createNamespace(ctx context.Context) error {
	namespace, err := harness.clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: harness.namespace},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating migration fault namespace: %w", err)
	}
	harness.namespaceUID = namespace.UID
	return nil
}

func (harness *clusterHarness) selectTarget(ctx context.Context, nodeName string) (cnsTarget, error) {
	pods, err := harness.clientset.CoreV1().Pods(kubeSystemNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: harness.labelSelector,
	})
	if err != nil {
		return cnsTarget{}, fmt.Errorf("listing CNS pods: %w", err)
	}
	pod, err := selectCNSTarget(pods.Items, nodeName)
	if err != nil {
		return cnsTarget{}, err
	}
	return targetFromPod(pod, harness.cnsContainer)
}

func targetFromPod(pod corev1.Pod, containerName string) (cnsTarget, error) {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName {
			if status.ContainerID == "" {
				return cnsTarget{}, fmt.Errorf("pod %s running CNS has no container ID", pod.Name)
			}
			return cnsTarget{
				PodName:     pod.Name,
				PodUID:      pod.UID,
				NodeName:    pod.Spec.NodeName,
				Container:   containerName,
				ContainerID: status.ContainerID,
				Restart:     status.RestartCount,
			}, nil
		}
	}
	return cnsTarget{}, fmt.Errorf("container %s was not found in CNS pod %s", containerName, pod.Name)
}

func newFaultControl(ctx context.Context, restConfig *rest.Config, target cnsTarget, token string) (*faultControl, error) {
	forwarder, err := integrationk8s.NewPortForwarder(restConfig, integrationk8s.PortForwardingOpts{
		Namespace: kubeSystemNamespace,
		PodName:   target.PodName,
		LocalPort: 0,
		DestPort:  cnsPort,
	})
	if err != nil {
		return nil, fmt.Errorf("creating CNS port forward: %w", err)
	}
	if err := forwarder.Forward(ctx); err != nil {
		return nil, fmt.Errorf("forwarding CNS port: %w", err)
	}
	return &faultControl{
		forwarder: forwarder,
		client:    &http.Client{Timeout: 10 * time.Second},
		baseURL:   forwarder.Address(),
		token:     token,
	}, nil
}

func (control *faultControl) close() {
	control.forwarder.Stop()
}

func (control *faultControl) arm(ctx context.Context, point string, target faultTarget) error {
	raw, err := json.Marshal(struct {
		Point  string      `json:"point"`
		Target faultTarget `json:"target"`
	}{
		Point:  point,
		Target: target,
	})
	if err != nil {
		return err
	}
	response, err := control.request(ctx, http.MethodPut, faultAPIPath, raw)
	if err != nil {
		return err
	}
	var status faultStatus
	if err := json.Unmarshal(response, &status); err != nil {
		return fmt.Errorf("decoding fault arm response: %w", err)
	}
	if status.Point != point || status.Target != target || status.State != "armed" {
		return fmt.Errorf(
			"unexpected fault arm status: point=%q target=%+v state=%q",
			status.Point,
			status.Target,
			status.State,
		)
	}
	return nil
}

func (control *faultControl) status(ctx context.Context) (faultStatus, error) {
	var status faultStatus
	response, err := control.request(ctx, http.MethodGet, faultAPIPath, nil)
	if err != nil {
		return status, err
	}
	if err := json.Unmarshal(response, &status); err != nil {
		return status, fmt.Errorf("decoding fault status: %w", err)
	}
	return status, nil
}

func (control *faultControl) waitReached(ctx context.Context) error {
	return wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, faultWaitTimeout, true, func(ctx context.Context) (bool, error) {
		status, err := control.status(ctx)
		if err != nil {
			return false, err
		}
		return status.State == "reached", nil
	})
}

func (control *faultControl) debug(ctx context.Context, path string, body []byte) ([]byte, error) {
	return control.request(ctx, http.MethodPost, path, body)
}

func (control *faultControl) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, control.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set(faultTokenHeader, control.token)
	if len(body) != 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := control.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("calling CNS %s: %w", path, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading CNS %s response: %w", path, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("request to CNS %s returned %s: %s", path, response.Status, raw)
	}
	return raw, nil
}

func (harness *clusterHarness) killTarget(ctx context.Context, target cnsTarget) error {
	err := harness.clientset.CoreV1().Pods(kubeSystemNamespace).Delete(
		ctx,
		target.PodName,
		exactPodDeleteOptions(target.PodUID),
	)
	if err != nil {
		return fmt.Errorf("deleting exact CNS pod %s/%s: %w", target.PodName, target.PodUID, err)
	}
	return nil
}

func (harness *clusterHarness) waitForReplacement(ctx context.Context, previous cnsTarget) (cnsTarget, error) {
	var replacement cnsTarget
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, rolloutWaitTimeout, true, func(ctx context.Context) (bool, error) {
		target, err := harness.selectTarget(ctx, previous.NodeName)
		if err != nil {
			return false, nil
		}
		if target.PodUID == previous.PodUID {
			return false, nil
		}
		replacement = target
		return true, nil
	})
	if err != nil {
		return cnsTarget{}, fmt.Errorf("waiting for CNS replacement on node %s: %w", previous.NodeName, err)
	}
	return replacement, nil
}

func (harness *clusterHarness) createPod(ctx context.Context, value scenario, nodeName string) (*corev1.Pod, error) {
	zero := int64(0)
	name := harness.workloadName(value)
	pod, err := harness.clientset.CoreV1().Pods(harness.namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: harness.workloadLabels(value),
		},
		Spec: corev1.PodSpec{
			NodeName:                      nodeName,
			RestartPolicy:                 corev1.RestartPolicyNever,
			TerminationGracePeriodSeconds: &zero,
			Containers: []corev1.Container{{
				Name:            "pause",
				Image:           harness.cfg.WorkloadImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
			}},
			NodeSelector: map[string]string{"kubernetes.io/os": harness.cfg.OS},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating workload pod: %w", err)
	}
	return pod, nil
}

func (harness *clusterHarness) createDeployment(ctx context.Context, value scenario, nodeName string) (*appsv1.Deployment, error) {
	replicas := int32(0)
	name := harness.workloadName(value)
	labels := harness.workloadLabels(value)
	deployment, err := harness.clientset.AppsV1().Deployments(harness.namespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					NodeName:      nodeName,
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{{
						Name:            "pause",
						Image:           harness.cfg.WorkloadImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
					}},
					NodeSelector: map[string]string{"kubernetes.io/os": harness.cfg.OS},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating scale deployment: %w", err)
	}
	return deployment, nil
}

func (harness *clusterHarness) scaleDeployment(ctx context.Context, name string, replicas int32) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := harness.clientset.AppsV1().Deployments(harness.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		deployment.Spec.Replicas = &replicas
		_, err = harness.clientset.AppsV1().Deployments(harness.namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	})
}

func (harness *clusterHarness) waitForWorkload(ctx context.Context, value scenario, work workload) error {
	switch value {
	case scenarioAddBeforeEndpointCommit, scenarioEndpointPatch:
		return harness.waitForPodReady(ctx, work.pod.Name)
	case scenarioDeleteAfterIntentCommit:
		return harness.waitForPodDeleted(ctx, work.pod.Name)
	case scenarioRestartDuringScale:
		return wait.PollUntilContextTimeout(ctx, 2*time.Second, rolloutWaitTimeout, true, func(ctx context.Context) (bool, error) {
			deployment, err := harness.clientset.AppsV1().Deployments(harness.namespace).Get(ctx, work.deployment.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return deployment.Status.AvailableReplicas == harness.cfg.ScaleReplicas, nil
		})
	default:
		return fmt.Errorf("unsupported migration fault scenario %q", value)
	}
}

func (harness *clusterHarness) waitForPodReady(ctx context.Context, name string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, rolloutWaitTimeout, true, func(ctx context.Context) (bool, error) {
		pod, err := harness.clientset.CoreV1().Pods(harness.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return isPodReady(*pod) && len(pod.Status.PodIPs) != 0, nil
	})
}

func (harness *clusterHarness) waitForPodDeleted(ctx context.Context, name string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, rolloutWaitTimeout, true, func(ctx context.Context) (bool, error) {
		_, err := harness.clientset.CoreV1().Pods(harness.namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (harness *clusterHarness) deletePodExact(ctx context.Context, pod corev1.Pod) error {
	return harness.clientset.CoreV1().Pods(harness.namespace).Delete(ctx, pod.Name, workloadPodDeleteOptions(pod.UID))
}

func (harness *clusterHarness) cleanupWorkload(ctx context.Context, work workload) error {
	if work.pod != nil {
		pod, err := harness.clientset.CoreV1().Pods(harness.namespace).Get(ctx, work.pod.Name, metav1.GetOptions{})
		switch {
		case err == nil:
			if err := harness.deletePodExact(ctx, *pod); err != nil {
				return err
			}
			return harness.waitForPodDeleted(ctx, pod.Name)
		case apierrors.IsNotFound(err):
		default:
			return err
		}
	}
	if work.deployment != nil {
		propagation := metav1.DeletePropagationForeground
		if err := harness.clientset.AppsV1().Deployments(harness.namespace).Delete(ctx, work.deployment.Name, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		selector := metav1.FormatLabelSelector(work.deployment.Spec.Selector)
		return wait.PollUntilContextTimeout(ctx, 2*time.Second, rolloutWaitTimeout, true, func(ctx context.Context) (bool, error) {
			_, err := harness.clientset.AppsV1().Deployments(harness.namespace).Get(ctx, work.deployment.Name, metav1.GetOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
			pods, listErr := harness.clientset.CoreV1().Pods(harness.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
			if listErr != nil {
				return false, listErr
			}
			return apierrors.IsNotFound(err) && len(pods.Items) == 0, nil
		})
	}
	return nil
}

func (harness *clusterHarness) workloadName(value scenario) string {
	return sanitizeResourceName("fi-"+string(value)+"-"+harness.cfg.RunID, 63)
}

func (harness *clusterHarness) workloadLabels(value scenario) map[string]string {
	return map[string]string{
		"acn.azure.com/migration-fault": sanitizeResourceName(harness.cfg.RunID, 63),
		"acn.azure.com/fault-scenario":  sanitizeResourceName(string(value), 63),
	}
}

func (harness *clusterHarness) captureArtifacts(
	ctx context.Context,
	dir, stage string,
	target cnsTarget,
	control *faultControl,
) error {
	stageDir := filepath.Join(dir, stage)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return err
	}
	var captureErrors []error
	if err := writeJSON(filepath.Join(stageDir, "target.json"), target); err != nil {
		captureErrors = append(captureErrors, err)
	}
	pods, err := harness.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + target.NodeName})
	if err != nil {
		captureErrors = append(captureErrors, err)
	} else if err := writeJSON(filepath.Join(stageDir, "pods.json"), pods); err != nil {
		captureErrors = append(captureErrors, err)
	}
	workloadPods, err := harness.clientset.CoreV1().Pods(harness.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		captureErrors = append(captureErrors, err)
	} else if err := writeJSON(filepath.Join(stageDir, "workload-pods.json"), workloadPods); err != nil {
		captureErrors = append(captureErrors, err)
	}
	logs, err := harness.clientset.CoreV1().Pods(kubeSystemNamespace).GetLogs(target.PodName, &corev1.PodLogOptions{
		Container:  target.Container,
		Timestamps: true,
	}).DoRaw(ctx)
	if err != nil {
		captureErrors = append(captureErrors, err)
	} else if err := writeFile(filepath.Join(stageDir, "cns.log"), logs); err != nil {
		captureErrors = append(captureErrors, err)
	}
	status, err := control.status(ctx)
	if err != nil {
		captureErrors = append(captureErrors, err)
	} else if err := writeJSON(filepath.Join(stageDir, "fault-status.json"), status); err != nil {
		captureErrors = append(captureErrors, err)
	}
	persistentRaw, err := control.debug(ctx, "/debug/persistentstate", []byte("{}"))
	if err != nil {
		captureErrors = append(captureErrors, err)
	} else {
		if err := writeFile(filepath.Join(stageDir, "persistentstate.json"), persistentRaw); err != nil {
			captureErrors = append(captureErrors, err)
		}
		if err := validatePersistentState(persistentRaw); err != nil {
			captureErrors = append(captureErrors, err)
		}
	}
	cacheRaw, err := control.debug(ctx, "/debug/ipaddresses", []byte(`{"IPConfigStateFilter":["Assigned"]}`))
	if err != nil {
		captureErrors = append(captureErrors, err)
	} else if err := writeFile(filepath.Join(stageDir, "ip-cache.json"), cacheRaw); err != nil {
		captureErrors = append(captureErrors, err)
	}
	if harness.cfg.OS == "windows" {
		hnsRaw, err := harness.captureHNS(ctx, target.NodeName)
		if err != nil {
			captureErrors = append(captureErrors, err)
		} else if err := writeFile(filepath.Join(stageDir, "hns-endpoints.json"), hnsRaw); err != nil {
			captureErrors = append(captureErrors, err)
		}
	}
	return errors.Join(captureErrors...)
}

func (harness *clusterHarness) captureHNS(ctx context.Context, nodeName string) ([]byte, error) {
	pods, err := acnk8s.GetPodsByNode(ctx, harness.clientset, kubeSystemNamespace, "app=privileged-daemonset", nodeName)
	if err != nil {
		return nil, err
	}
	pod, err := selectCNSTarget(pods.Items, nodeName)
	if err != nil {
		return nil, fmt.Errorf("selecting Windows privileged pod: %w", err)
	}
	stdout, stderr, err := acnk8s.ExecCmdOnPod(ctx, harness.clientset, kubeSystemNamespace, pod.Name, "powershell", []string{
		"powershell",
		"-NoProfile",
		"-Command",
		"Get-HnsEndpoint | ConvertTo-Json -Depth 20",
	}, harness.restConfig, true)
	if err != nil {
		return nil, fmt.Errorf("capturing HNS endpoints: %w: %s", err, stderr)
	}
	return stdout, nil
}

func (harness *clusterHarness) captureFailure(ctx context.Context, dir string, target cnsTarget) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	current, err := harness.selectTarget(ctx, target.NodeName)
	if err != nil {
		_ = writeFile(filepath.Join(dir, "failure-capture-error.txt"), []byte(err.Error()))
		return
	}
	control, err := newFaultControl(ctx, harness.restConfig, current, harness.token)
	if err != nil {
		_ = writeFile(filepath.Join(dir, "failure-capture-error.txt"), []byte(err.Error()))
		return
	}
	defer control.close()
	if err := harness.captureArtifacts(ctx, dir, "failure", current, control); err != nil {
		_ = writeFile(filepath.Join(dir, "failure-capture-error.txt"), []byte(err.Error()))
	}
}

func validatePersistentState(raw []byte) error {
	var response persistentstate.DebugResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("decoding persistent state response: %w", err)
	}
	if response.Storage.Backend != persistentstate.StorageBackendBolt {
		return fmt.Errorf("unexpected persistent state backend %q", response.Storage.Backend)
	}
	if !response.Storage.FilePresent || response.Storage.FileSizeBytes <= 0 {
		return fmt.Errorf("persistent state database file is unavailable")
	}
	if err := response.Snapshot.Validate(); err != nil {
		return fmt.Errorf("validating persistent state snapshot: %w", err)
	}
	return nil
}

func validationResult(err error) string {
	if err == nil {
		return "persistent state, IP cache, live pod, and platform checks passed\n"
	}
	return "validation failed: " + err.Error() + "\n"
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, raw)
}

func writeFile(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
