package kubernetes

import (
	"context"
	"log"
	"time"

	"github.com/Azure/azure-container-networking/test/internal/retry"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

// ownedBy reports whether the pod has an owner reference with the given UID.
func ownedBy(pod *corev1.Pod, ownerUID types.UID) bool {
	for index := range pod.OwnerReferences {
		if pod.OwnerReferences[index].UID == ownerUID {
			return true
		}
	}
	return false
}

// selectorString builds a label selector string from a DaemonSet's spec selector.
func selectorString(daemonset *appsv1.DaemonSet) (string, error) {
	selector, err := metav1.LabelSelectorAsSelector(daemonset.Spec.Selector)
	if err != nil {
		return "", errors.Wrapf(err, "could not build selector for daemonset %s", daemonset.Name)
	}
	return selector.String(), nil
}

// listNotRunningDaemonsetPods returns the pods owned by the DaemonSet (matched by
// selector and owner UID, so CronJob pods sharing the label are excluded) whose
// phase is not Running.
func listNotRunningDaemonsetPods(ctx context.Context, clientset *kubernetes.Clientset, namespace, selector string, ownerUID types.UID) ([]corev1.Pod, error) {
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, errors.Wrapf(err, "could not list pods with label selector %s", selector)
	}
	var notRunning []corev1.Pod
	for index := range podList.Items {
		pod := &podList.Items[index]
		if !ownedBy(pod, ownerUID) {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			notRunning = append(notRunning, *pod)
		}
	}
	return notRunning, nil
}

// RecoverDaemonsetAfterRestart is a post-node-restart readiness gate. It waits
// for all nodes to be Ready, then - only if a pod owned by the named DaemonSet is
// not yet Running - waits settle for it to finish its post-reboot init (image
// pulls and package installs can legitimately take minutes) before deleting any
// of its pods still not Running. A pod stuck that long is wedged in a saturated
// CrashLoopBackOff; recreating it resets the back-off timer and gives it a fresh
// sandbox. Finally it waits for the DaemonSet to be fully rolled out.
//
// The pod selector is read from the DaemonSet's own spec (not hard-coded) and pod
// operations are scoped to pods owned by the DaemonSet, so unrelated pods sharing
// the label (e.g. completed CronJob cleanup pods) are never touched and never
// trigger the settle. Healthy clusters therefore incur no extra delay.
func RecoverDaemonsetAfterRestart(ctx context.Context, clientset *kubernetes.Clientset, namespace, daemonsetName string, settle time.Duration) error {
	log.Print("Waiting for all nodes to be Ready after node restart")
	if err := WaitForNodesReady(ctx, clientset); err != nil {
		return errors.Wrap(err, "nodes did not become ready after restart")
	}

	daemonset, err := clientset.AppsV1().DaemonSets(namespace).Get(ctx, daemonsetName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("daemonset %s/%s not found; nothing to recover", namespace, daemonsetName)
			return nil
		}
		return errors.Wrapf(err, "could not get daemonset %s", daemonsetName)
	}
	selector, err := selectorString(daemonset)
	if err != nil {
		return err
	}

	notRunning, err := listNotRunningDaemonsetPods(ctx, clientset, namespace, selector, daemonset.UID)
	if err != nil {
		return errors.Wrap(err, "could not check daemonset pod state")
	}

	if len(notRunning) > 0 {
		log.Printf("%d %s pod(s) not Running; waiting %s for them to settle before forcing a restart", len(notRunning), daemonsetName, settle)
		select {
		case <-time.After(settle):
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "context cancelled while waiting for daemonset to settle")
		}

		stillWedged, err := listNotRunningDaemonsetPods(ctx, clientset, namespace, selector, daemonset.UID)
		if err != nil {
			return errors.Wrap(err, "could not re-check daemonset pod state")
		}
		podsClient := clientset.CoreV1().Pods(namespace)
		for index := range stillWedged {
			name := stillWedged[index].Name
			log.Printf("Deleting non-Running pod %s/%s (phase %s) to break CrashLoopBackOff", namespace, name, stillWedged[index].Status.Phase)
			if err := podsClient.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "could not delete pod %s", name)
			}
		}
	}

	log.Printf("Waiting for daemonset %s to be rolled out", daemonsetName)
	return WaitForDaemonsetReady(ctx, clientset, namespace, daemonsetName)
}
