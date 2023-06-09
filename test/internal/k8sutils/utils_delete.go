package k8sutils

import (
	"context"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
)

func mustDeleteDaemonset(ctx context.Context, daemonsets typedappsv1.DaemonSetInterface, ds appsv1.DaemonSet) error {
	if err := daemonsets.Delete(ctx, ds.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func mustDeleteDeployment(ctx context.Context, deployments typedappsv1.DeploymentInterface, d appsv1.Deployment) error {
	if err := deployments.Delete(ctx, d.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func MustDeleteNamespace(ctx context.Context, clienset *kubernetes.Clientset, namespace string) error {
	if err := clienset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete namespace %v", namespace)
		}
	}
	return nil
}
