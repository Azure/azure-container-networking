package kubernetes

import (
	"context"
	"log"
	"time"

	"github.com/Azure/azure-container-networking/test/internal/retry"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// isNodeReady reports whether a node's Ready condition is True.
func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// WaitForNodesReady blocks until every node reports a Ready condition of True.
func WaitForNodesReady(ctx context.Context, clientset *kubernetes.Clientset) error {
	checkNodesReadyFn := func() error {
		nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return errors.Wrap(err, "could not list nodes")
		}
		if len(nodes.Items) == 0 {
			return errors.New("no nodes found")
		}
		for index := range nodes.Items {
			if !isNodeReady(&nodes.Items[index]) {
				return errors.Errorf("node %s is not ready", nodes.Items[index].Name)
			}
		}
		return nil
	}

	retrier := retry.Retrier{Attempts: RetryAttempts, Delay: RetryDelay}
	return errors.Wrap(retrier.Do(ctx, checkNodesReadyFn), "failed to wait for nodes to be ready")
}

// WaitForDaemonsetReady blocks until the named DaemonSet is fully rolled out,
// mirroring `kubectl rollout status`. It compares the DaemonSet's own status
// counters (rather than counting labelled pods) so unrelated pods that happen to
// share the label, e.g. completed CronJob pods, do not skew the result.
func WaitForDaemonsetReady(ctx context.Context, clientset *kubernetes.Clientset, namespace, daemonsetName string) error {
	daemonsetClient := clientset.AppsV1().DaemonSets(namespace)
	checkDaemonsetReadyFn := func() error {
		daemonset, err := daemonsetClient.Get(ctx, daemonsetName, metav1.GetOptions{})
		if err != nil {
			return errors.Wrapf(err, "could not get daemonset %s", daemonsetName)
		}
		desired := daemonset.Status.DesiredNumberScheduled
		if desired == 0 {
			return errors.Errorf("daemonset %s reports 0 desired pods", daemonsetName)
		}
		if daemonset.Status.UpdatedNumberScheduled != desired || daemonset.Status.NumberAvailable != desired {
			log.Printf("daemonset %s not fully rolled out: updated %d/%d, available %d/%d",
				daemonsetName, daemonset.Status.UpdatedNumberScheduled, desired, daemonset.Status.NumberAvailable, desired)
			return errors.Errorf("daemonset %s is not fully rolled out", daemonsetName)
		}
		return nil
	}

	retrier := retry.Retrier{Attempts: RetryAttempts, Delay: RetryDelay}
	return errors.Wrapf(retrier.Do(ctx, checkDaemonsetReadyFn), "daemonset %s did not become ready", daemonsetName)
}

// deleteNotRunningPods deletes every pod matching the label selector whose phase
// is not Running. Missing pods are ignored so the call is idempotent.
func deleteNotRunningPods(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelSelector string) error {
	podsClient := clientset.CoreV1().Pods(namespace)
	podList, err := podsClient.List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return errors.Wrapf(err, "could not list pods with label selector %s", labelSelector)
	}
	for index := range podList.Items {
		pod := podList.Items[index]
		if pod.Status.Phase == corev1.PodRunning {
			continue
		}
		log.Printf("Deleting non-Running pod %s/%s (phase %s) to break CrashLoopBackOff", namespace, pod.Name, pod.Status.Phase)
		if err := podsClient.Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "could not delete pod %s", pod.Name)
		}
	}
	return nil
}

// hasNotRunningPod reports whether any pod matching the label selector is not in
// the Running phase.
func hasNotRunningPod(ctx context.Context, clientset *kubernetes.Clientset, namespace, labelSelector string) (bool, error) {
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return false, errors.Wrapf(err, "could not list pods with label selector %s", labelSelector)
	}
	for index := range podList.Items {
		if podList.Items[index].Status.Phase != corev1.PodRunning {
			return true, nil
		}
	}
	return false, nil
}

// RecoverDaemonsetAfterRestart is a post-node-restart readiness gate. It waits
// for all nodes to be Ready, then - only if a pod of the named DaemonSet is not
// yet Running - waits `settle` for it to finish its post-reboot init (image
// pulls and package installs can legitimately take minutes) before deleting any
// pod still not Running. A pod stuck that long is wedged in a saturated
// CrashLoopBackOff; recreating it resets the back-off timer and gives it a fresh
// sandbox. Finally it waits for the DaemonSet to be fully rolled out. Healthy
// clusters incur no extra delay because the settle only runs when a pod is not
// Running.
func RecoverDaemonsetAfterRestart(ctx context.Context, clientset *kubernetes.Clientset, namespace, daemonsetName, labelSelector string, settle time.Duration) error {
	log.Print("Waiting for all nodes to be Ready after node restart")
	if err := WaitForNodesReady(ctx, clientset); err != nil {
		return errors.Wrap(err, "nodes did not become ready after restart")
	}

	notRunning, err := hasNotRunningPod(ctx, clientset, namespace, labelSelector)
	if err != nil {
		return errors.Wrap(err, "could not check daemonset pod state")
	}

	if notRunning {
		log.Printf("A %s pod is not Running; waiting %s for it to settle before forcing a restart", daemonsetName, settle)
		select {
		case <-time.After(settle):
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "context cancelled while waiting for daemonset to settle")
		}
		if err := deleteNotRunningPods(ctx, clientset, namespace, labelSelector); err != nil {
			return errors.Wrapf(err, "could not delete wedged %s pods", daemonsetName)
		}
	}

	log.Printf("Waiting for daemonset %s to be rolled out", daemonsetName)
	return WaitForDaemonsetReady(ctx, clientset, namespace, daemonsetName)
}
