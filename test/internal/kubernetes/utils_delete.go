package kubernetes

import (
	"context"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	typedciliumv2 "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/typed/cilium.io/v2"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func MustDeletePod(ctx context.Context, podI typedcorev1.PodInterface, pod corev1.Pod) {
	if err := podI.Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete pod"))
		}
	}
}

func MustDeleteDaemonset(ctx context.Context, daemonsets typedappsv1.DaemonSetInterface, ds appsv1.DaemonSet) {
	if err := daemonsets.Delete(ctx, ds.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete daemonset"))
		}
	}
}

func MustDeleteDeployment(ctx context.Context, deployments typedappsv1.DeploymentInterface, d appsv1.Deployment) {
	if err := deployments.Delete(ctx, d.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete deployment"))
		}
	}
	if err := WaitForDeploymentToDelete(ctx, deployments, d); err != nil {
		panic(errors.Wrap(err, "failed to wait for deployment to delete"))
	}
}

func MustDeleteNamespace(ctx context.Context, clienset *kubernetes.Clientset, namespace string) {
	if err := clienset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrapf(err, "failed to delete namespace %v", namespace))
		}
	}
}

func MustDeleteConfigMap(ctx context.Context, configMaps typedcorev1.ConfigMapInterface, cm corev1.ConfigMap) {
	if err := configMaps.Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete config map"))
		}
	}
}

func MustDeleteServiceAccount(ctx context.Context, serviceAccounts typedcorev1.ServiceAccountInterface, svcAcct corev1.ServiceAccount) {
	if err := serviceAccounts.Delete(ctx, svcAcct.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete service account"))
		}
	}
}

func MustDeleteService(ctx context.Context, services typedcorev1.ServiceInterface, svc corev1.Service) {
	if err := services.Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete service"))
		}
	}
}

func MustDeleteCiliumLocalRedirectPolicy(ctx context.Context, lrpClient typedciliumv2.CiliumLocalRedirectPolicyInterface, clrp ciliumv2.CiliumLocalRedirectPolicy) {
	if err := lrpClient.Delete(ctx, clrp.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete cilium local redirect policy"))
		}
	}
}

func MustDeleteCiliumNetworkPolicy(ctx context.Context, cnpClient typedciliumv2.CiliumNetworkPolicyInterface, cnp ciliumv2.CiliumNetworkPolicy) {
	if err := cnpClient.Delete(ctx, cnp.Name, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			panic(errors.Wrap(err, "failed to delete cilium network policy"))
		}
	}
}
