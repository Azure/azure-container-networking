package helpers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	mtv1alpha1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errDeploymentNotReady  = errors.New("deployment not ready")
	errDeploymentPodsExist = errors.New("deployment pods not fully removed")
	errDaemonSetNotReady   = errors.New("daemonset not ready")
	errMTPNCStillPresent   = errors.New("MTPNCs still present")
)

const (
	LongRunningCreatedAtAnnotation = "acn-test/created-at"
)

// LongrunningDeploymentData contains configuration for creating a deployment in long-running tests.
type LongrunningDeploymentData struct {
	DeploymentName string
	Namespace      string
	PNIName        string
	PNName         string
	NodeName       string
	Image          string
	CreatedAt      string
}

// LongrunningDaemonSetData contains configuration for creating a daemonset in long-running tests.
type LongrunningDaemonSetData struct {
	DaemonSetName string
	Namespace     string
	PNIName       string
	PNName        string
	ZoneLabel     string
	Image         string
}

var (
	clientCache   = make(map[string]client.Client) //nolint:gochecknoglobals // cached test clients
	clientCacheMu sync.Mutex                       //nolint:gochecknoglobals // guards clientCache
)

// MustGetK8sClient returns a controller-runtime client for the given kubeconfig.
// Clients are cached per kubeconfig path. Panics on error since test helpers
// are only called from Ginkgo specs where a panic is equivalent to a test failure.
func MustGetK8sClient(kubeconfig string) client.Client {
	clientCacheMu.Lock()
	defer clientCacheMu.Unlock()

	if c, ok := clientCache[kubeconfig]; ok {
		return c
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add client-go scheme to runtime scheme: %v", err))
	}
	if err := mtv1alpha1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add multitenancy v1alpha1 scheme to runtime scheme: %v", err))
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(fmt.Sprintf("failed to build kubeconfig from %s: %v", kubeconfig, err))
	}

	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		panic(fmt.Sprintf("failed to create k8s client from %s: %v", kubeconfig, err))
	}

	clientCache[kubeconfig] = c
	return c
}

// WithRetry executes fn up to maxRetries times with a fixed delay between attempts.
// It returns immediately on success or non-retryable errors (NotFound, AlreadyExists).
func WithRetry(ctx context.Context, maxRetries int, delay time.Duration, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if apierrors.IsNotFound(lastErr) || apierrors.IsAlreadyExists(lastErr) {
			return lastErr
		}
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
		}
	}
	return lastErr
}

// --- Namespace helpers ---

func EnsureNamespaceK8s(ctx context.Context, c client.Client, namespace string) error {
	ns := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: namespace}, ns)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check namespace %s: %w", namespace, err)
	}
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	if err := c.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}
	return nil
}

// --- Node helpers ---

func GetNodeByLabels(ctx context.Context, c client.Client, labelSelector string) (string, error) {
	labels, err := metav1.ParseToLabelSelector(labelSelector)
	if err != nil {
		return "", fmt.Errorf("failed to parse label selector %q: %w", labelSelector, err)
	}
	sel, err := metav1.LabelSelectorAsSelector(labels)
	if err != nil {
		return "", fmt.Errorf("failed to convert label selector: %w", err)
	}

	nodeList := &corev1.NodeList{}
	if err := c.List(ctx, nodeList, client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return "", fmt.Errorf("failed to list nodes with selector %q: %w", labelSelector, err)
	}
	if len(nodeList.Items) == 0 {
		return "", nil
	}
	return nodeList.Items[0].Name, nil
}

func GetNodeZoneLabel(ctx context.Context, c client.Client, nodeName string) (string, error) {
	node := &corev1.Node{}
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}
	return node.Labels["topology.kubernetes.io/zone"], nil
}

// --- Deployment helpers ---

func DeploymentExists(ctx context.Context, c client.Client, namespace, name string) (bool, error) {
	dep := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, dep)
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check deployment %s: %w", name, err)
}

func IsDeploymentReady(ctx context.Context, c client.Client, namespace, name string) (bool, error) {
	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, dep); err != nil {
		return false, fmt.Errorf("failed to get deployment %s: %w", name, err)
	}
	return dep.Status.ReadyReplicas >= 1, nil
}

func GetDeploymentPodName(ctx context.Context, c client.Client, namespace, deploymentName string) (string, error) {
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels{"app": deploymentName}); err != nil {
		return "", fmt.Errorf("failed to list pods for deployment %s: %w", deploymentName, err)
	}
	if len(podList.Items) == 0 {
		return "", nil
	}
	return podList.Items[0].Name, nil
}

func DeleteDeploymentAndWait(ctx context.Context, c client.Client, namespace, name string, timeout time.Duration) error {
	dep := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, dep)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get deployment %s: %w", name, err)
	}

	if err := c.Delete(ctx, dep); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment %s: %w", name, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		podList := &corev1.PodList{}
		if err := c.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels{"app": name}); err != nil {
			return fmt.Errorf("failed to list pods for deployment %s: %w", name, err)
		}
		if len(podList.Items) == 0 {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("%w: deployment %s after %v", errDeploymentPodsExist, name, timeout)
}

func WaitForDeploymentReady(ctx context.Context, c client.Client, namespace, name string, maxRetries, sleepSeconds int) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		ready, err := IsDeploymentReady(ctx, c, namespace, name)
		if err == nil && ready {
			fmt.Printf("Deployment %s has ready replica(s)\n", name)
			return nil
		}

		if attempt < maxRetries {
			fmt.Printf("Deployment %s not ready yet (attempt %d/%d). Waiting %d seconds...\n",
				name, attempt, maxRetries, sleepSeconds)
			time.Sleep(time.Duration(sleepSeconds) * time.Second)
		}
	}
	return fmt.Errorf("%w: deployment %s after %d attempts", errDeploymentNotReady, name, maxRetries)
}

// --- DaemonSet helpers ---

func DaemonSetExists(ctx context.Context, c client.Client, namespace, name string) (bool, error) {
	ds := &appsv1.DaemonSet{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, ds)
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check daemonset %s: %w", name, err)
}

func GetDaemonSetPodName(ctx context.Context, c client.Client, namespace, dsName string) (string, error) {
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels{"app": dsName}); err != nil {
		return "", fmt.Errorf("failed to list pods for daemonset %s: %w", dsName, err)
	}
	if len(podList.Items) == 0 {
		return "", nil
	}
	return podList.Items[0].Name, nil
}

func WaitForDaemonSetReady(ctx context.Context, c client.Client, namespace, dsName string, maxRetries, sleepSeconds int) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		ds := &appsv1.DaemonSet{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: dsName}, ds); err == nil {
			if ds.Status.NumberReady >= 1 {
				fmt.Printf("DaemonSet %s has %d ready pod(s)\n", dsName, ds.Status.NumberReady)
				return nil
			}
		}

		if attempt < maxRetries {
			fmt.Printf("DaemonSet %s not ready yet (attempt %d/%d). Waiting %d seconds...\n",
				dsName, attempt, maxRetries, sleepSeconds)
			time.Sleep(time.Duration(sleepSeconds) * time.Second)
		}
	}
	return fmt.Errorf("%w: daemonset %s after %d attempts", errDaemonSetNotReady, dsName, maxRetries)
}

// --- CRD helpers (PodNetwork, PodNetworkInstance, MTPNC) ---

func PodNetworkExists(ctx context.Context, c client.Client, name string) (bool, error) {
	pn := &mtv1alpha1.PodNetwork{}
	err := c.Get(ctx, types.NamespacedName{Name: name}, pn)
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check PodNetwork %s: %w", name, err)
}

func CreatePodNetworkCR(ctx context.Context, c client.Client, name, vnetGUID, subnetGUID, subnetARMID string) error {
	pn := &mtv1alpha1.PodNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: mtv1alpha1.PodNetworkSpec{
			VnetGUID:         vnetGUID,
			SubnetGUID:       subnetGUID,
			SubnetResourceID: subnetARMID,
			NetworkID:        vnetGUID,
			DeviceType:       mtv1alpha1.DeviceTypeVnetNIC,
		},
	}
	if err := c.Create(ctx, pn); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create PodNetwork %s: %w", name, err)
	}
	return nil
}

func PodNetworkInstanceExists(ctx context.Context, c client.Client, namespace, name string) (bool, error) {
	pni := &mtv1alpha1.PodNetworkInstance{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, pni)
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check PodNetworkInstance %s: %w", name, err)
}

func CreatePodNetworkInstanceCR(ctx context.Context, c client.Client, name, namespace, pnName string, reservations int) error {
	pni := &mtv1alpha1.PodNetworkInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: mtv1alpha1.PodNetworkInstanceSpec{
			PodNetworkConfigs: []mtv1alpha1.PodNetworkConfig{
				{
					PodNetwork:           pnName,
					PodIPReservationSize: reservations,
				},
			},
		},
	}
	if err := c.Create(ctx, pni); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create PodNetworkInstance %s: %w", name, err)
	}
	return nil
}

func WaitForMTPNCCleanupK8s(ctx context.Context, c client.Client, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mtpncList := &mtv1alpha1.MultitenantPodNetworkConfigList{}
		if err := c.List(ctx, mtpncList, client.InNamespace(namespace)); err != nil {
			// CRD not installed — no MTPNCs can exist
			if apierrors.IsNotFound(err) || isNoCRDError(err) {
				return nil
			}
			fmt.Printf("Warning: failed to list MTPNCs in namespace %s: %v\n", namespace, err)
			time.Sleep(5 * time.Second)
			continue
		}
		if len(mtpncList.Items) == 0 {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("%w in namespace %s after %v", errMTPNCStillPresent, namespace, timeout)
}

func isNoCRDError(err error) bool {
	// The API server returns a specific error when a CRD is not installed
	return apierrors.IsNotFound(err) ||
		(err != nil && (apierrors.ReasonForError(err) == metav1.StatusReasonNotFound))
}

// --- Deployment creation (programmatic, replaces template) ---

func CreateDeploymentObject(data LongrunningDeploymentData) *appsv1.Deployment {
	replicas := int32(1)
	privileged := true
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      data.DeploymentName,
			Namespace: data.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": data.DeploymentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": data.DeploymentName,
						"kubernetes.azure.com/pod-network-instance": data.PNIName,
						"kubernetes.azure.com/pod-network":          data.PNName,
					},
					Annotations: map[string]string{
						LongRunningCreatedAtAnnotation: data.CreatedAt,
					},
				},
				Spec: podSpec(data.NodeName, data.Image, privileged),
			},
		},
	}
}

func CreateDaemonSetObject(data LongrunningDaemonSetData) *appsv1.DaemonSet {
	privileged := true
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      data.DaemonSetName,
			Namespace: data.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": data.DaemonSetName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": data.DaemonSetName,
						"kubernetes.azure.com/pod-network-instance": data.PNIName,
						"kubernetes.azure.com/pod-network":          data.PNName,
					},
				},
				Spec: daemonSetPodSpec(data.ZoneLabel, data.Image, privileged),
			},
		},
	}
}

func podSpec(nodeName, image string, privileged bool) corev1.PodSpec {
	return corev1.PodSpec{
		NodeSelector: map[string]string{
			"kubernetes.io/hostname": nodeName,
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      "acn-test/zone-pool",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
		Containers: []corev1.Container{
			netDebuggerContainer(image, privileged),
		},
		RestartPolicy: corev1.RestartPolicyAlways,
	}
}

func daemonSetPodSpec(zoneLabel, image string, privileged bool) corev1.PodSpec {
	return corev1.PodSpec{
		NodeSelector: map[string]string{
			"longrunning-zone-pool":       "true",
			"topology.kubernetes.io/zone": zoneLabel,
		},
		Tolerations: []corev1.Toleration{
			{
				Key:      "acn-test/zone-pool",
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
		Containers: []corev1.Container{
			netDebuggerContainer(image, privileged),
		},
		RestartPolicy: corev1.RestartPolicyAlways,
	}
}

func netDebuggerContainer(image string, privileged bool) corev1.Container {
	return corev1.Container{
		Name:    "net-debugger",
		Image:   image,
		Command: []string{"/bin/bash", "-c"},
		Args: []string{
			`echo "Pod Network Diagnostics started on $(hostname)"
echo "Pod IP: $(hostname -i)"
echo "Starting TCP listener on port 8080"
while true; do
  echo "TCP Connection Success from $(hostname) at $(date)" | nc -l -p 8080
done`,
		},
		Ports: []corev1.ContainerPort{
			{ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
		},
		Resources: vnetNICResources(),
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
	}
}

func vnetNICResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:       mustParseQuantity("300m"),
			corev1.ResourceMemory:    mustParseQuantity("600Mi"),
			"acn.azure.com/vnet-nic": mustParseQuantity("1"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:       mustParseQuantity("300m"),
			corev1.ResourceMemory:    mustParseQuantity("600Mi"),
			"acn.azure.com/vnet-nic": mustParseQuantity("1"),
		},
	}
}

func mustParseQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse quantity %q: %v", s, err))
	}
	return q
}
